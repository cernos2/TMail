<script setup lang="ts">
// Message body editor (contenteditable). Images pasted from the clipboard or dropped onto the text are
// inserted where the caret is; other pasted or dropped files become attachments. Pasted HTML is reduced
// to simple formatting so text from Word or web pages keeps bold/lists/tables but not their styles.
// A clicked image gets a frame with a corner handle and size presets.
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { toast } from '~/composables/toast'
import { aiName, type AssistKind } from '~/composables/ai'
import { t } from '~/composables/i18n'
import { WORD_RE, checkWords, isKnown, isWrong, suggestions, skipWord, addToDict, pairFixes, isPairKnown } from '~/composables/spell'

// assist: the right-click menu also offers dictation and Loggi (the letter editor, not signatures).
const props = defineProps<{ modelValue: string, placeholder?: string, addImage: (f: File) => { cid: string, url: string }, assist?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string], files: [File[]], assist: [kind: AssistKind, text: string] }>()
const el = ref<HTMLDivElement | null>(null)
const MAX_IMAGE = 10 * 1024 * 1024

// Quoted letters and drafts come from outside: the editor lives in our page, so <style> blocks
// (which would restyle the whole webmail), scripts, frames and event handlers are dropped.
function setHTML(v: string) {
  const doc = new DOMParser().parseFromString(v, 'text/html')
  doc.body.querySelectorAll('style,script,link,meta,iframe,frame,object,embed,form,base').forEach(n => n.remove())
  doc.body.querySelectorAll('*').forEach(n => { for (const a of [...n.attributes]) if (/^on/i.test(a.name) || /^\s*javascript:/i.test(a.value)) n.removeAttribute(a.name) })
  el.value!.innerHTML = doc.body.innerHTML
}

onMounted(() => {
  if (el.value) setHTML(props.modelValue)
  document.addEventListener('selectionchange', onSelection)
  window.addEventListener('resize', placeFrame)
  window.addEventListener('scroll', placeFrame, true)
  window.addEventListener('tm-spell', repaint)
  spellSoon(0)
})
onBeforeUnmount(() => {
  document.removeEventListener('selectionchange', onSelection)
  window.removeEventListener('resize', placeFrame)
  window.removeEventListener('scroll', placeFrame, true)
  window.removeEventListener('tm-spell', repaint)
  clearTimeout(spellT); clearTimeout(tipT)
  unpaint()
})
watch(() => props.modelValue, v => { if (el.value && v !== el.value.innerHTML) { setHTML(v); selImg.value = null; spellSoon(300) } })

const sync = () => emit('update:modelValue', el.value?.innerHTML || '')
const esc = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]!))

// ---------- selection and formatting commands ----------

let saved: Range | null = null
const fmt = reactive<Record<string, boolean>>({})
const FMT = ['bold', 'italic', 'underline', 'strikeThrough', 'insertOrderedList', 'insertUnorderedList', 'justifyLeft', 'justifyCenter', 'justifyRight']
function onSelection() {
  const sel = window.getSelection()
  if (!sel?.rangeCount || !el.value?.contains(sel.anchorNode)) return
  saved = sel.getRangeAt(0).cloneRange()
  for (const c of FMT) fmt[c] = document.queryCommandState(c)
  spellSoon(400)
}
function restore() {
  const n = el.value
  if (!n) return
  n.focus()
  const sel = window.getSelection()
  if (saved && n.contains(saved.startContainer)) { sel?.removeAllRanges(); sel?.addRange(saved) }
}
function exec(cmd: string, value?: string) {
  restore()
  document.execCommand('styleWithCSS', false, 'false')
  document.execCommand(cmd, false, value)
  onSelection()
  sync()
}
function insertLink(url: string, text: string) {
  restore()
  const sel = window.getSelection()
  const href = /^(https?:|mailto:|tel:)/i.test(url) ? url : /@/.test(url) && !/\//.test(url) ? `mailto:${url}` : `https://${url}`
  if (sel && !sel.isCollapsed && !text) document.execCommand('createLink', false, href)
  else document.execCommand('insertHTML', false, `<a href="${esc(href)}">${esc(text || url)}</a>&nbsp;`)
  sync()
}
const selectedText = () => (saved && !saved.collapsed ? saved.toString() : '')

function insertImages(files: File[]) {
  for (const f of files) {
    if (f.size > MAX_IMAGE) { toast(t('editor.imageTooBig', { name: f.name || '' }), { kind: 'err' }); continue }
    const { cid, url } = props.addImage(f)
    document.execCommand('insertHTML', false,
      `<img src="${url}" data-cid="${cid}" alt="${esc(f.name || t('editor.imageAlt'))}" style="max-width:100%;height:auto">`)
  }
  sync()
}
function insertImageFiles(files: File[]) { restore(); insertImages(files) }

// ---------- paste and drop ----------

const KEEP = new Set(['P', 'DIV', 'BR', 'B', 'STRONG', 'I', 'EM', 'U', 'S', 'A', 'UL', 'OL', 'LI', 'H1', 'H2', 'H3', 'H4',
  'TABLE', 'THEAD', 'TBODY', 'TR', 'TD', 'TH', 'BLOCKQUOTE', 'SPAN', 'PRE', 'CODE'])
function cleanPaste(html: string) {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('script,style,meta,link,title,img,svg,video,audio,iframe,object,form,input,button').forEach(n => n.remove())
  const walk = (node: Element) => {
    for (const child of Array.from(node.children)) {
      walk(child)
      if (!KEEP.has(child.tagName)) { child.replaceWith(...Array.from(child.childNodes)); continue }
      const href = child.tagName === 'A' ? child.getAttribute('href') || '' : ''
      for (const a of Array.from(child.attributes)) child.removeAttribute(a.name)
      if (/^(https?:|mailto:)/i.test(href)) child.setAttribute('href', href)
    }
  }
  walk(doc.body)
  return doc.body.innerHTML
}

const isImage = (f: File) => /^image\/(png|jpe?g|gif|webp|bmp)$/.test(f.type)
function onPaste(e: ClipboardEvent) {
  const dt = e.clipboardData
  if (!dt) return
  e.stopPropagation()
  // Files copied in Explorer/Finder arrive in dt.files; screenshots as image items.
  let files = Array.from(dt.files)
  if (!files.length) files = Array.from(dt.items).filter(i => i.kind === 'file').map(i => i.getAsFile()).filter(Boolean) as File[]
  if (files.length) {
    e.preventDefault()
    const images = files.filter(isImage)
    const others = files.filter(f => !isImage(f))
    if (images.length) insertImages(images)
    if (others.length) { emit('files', others); toast(others.length === 1 ? t('editor.fileAttached', { name: others[0].name }) : t('editor.filesAttached', { n: others.length })) }
    return
  }
  const html = dt.getData('text/html')
  if (html) { e.preventDefault(); document.execCommand('insertHTML', false, cleanPaste(html)); sync(); return }
  const text = dt.getData('text/plain')
  if (text) { e.preventDefault(); document.execCommand('insertText', false, text); sync() }
}

function onDrop(e: DragEvent) {
  const files = Array.from(e.dataTransfer?.files || [])
  if (!files.length) return
  e.preventDefault()
  e.stopPropagation()
  // Put the caret where the files were dropped.
  const r = (document as any).caretRangeFromPoint?.(e.clientX, e.clientY) as Range | undefined
  if (r) { const sel = window.getSelection(); sel?.removeAllRanges(); sel?.addRange(r) }
  const images = files.filter(isImage)
  const others = files.filter(f => !isImage(f))
  if (images.length) { el.value?.focus(); insertImages(images) }
  if (others.length) emit('files', others)
}

// ---------- image resizing ----------

const selImg = ref<HTMLImageElement | null>(null)
const frame = ref<Record<string, string>>({})
const imgWidth = ref(0)

function placeFrame() {
  const img = selImg.value
  if (!img) return
  if (!el.value?.contains(img)) { selImg.value = null; return }
  const r = img.getBoundingClientRect()
  const box = el.value.getBoundingClientRect()
  // Hide the frame when the image is scrolled out of the editor.
  const hidden = r.bottom < box.top || r.top > box.bottom
  frame.value = { left: `${r.left}px`, top: `${r.top}px`, width: `${r.width}px`, height: `${r.height}px`, display: hidden ? 'none' : 'block' }
  imgWidth.value = Math.round(r.width)
}
function onClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.tagName === 'IMG') {
    selImg.value = target as HTMLImageElement
    const r = document.createRange()
    r.selectNode(target)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(r)
    placeFrame()
  } else selImg.value = null
}
function onKeydown(e: KeyboardEvent) {
  if (!selImg.value) return
  if (e.key === 'Delete' || e.key === 'Backspace') { e.preventDefault(); removeImage(); return }
  selImg.value = null
}
const maxWidth = () => (el.value ? el.value.clientWidth - 28 : 600)
function setWidth(w: number) {
  const img = selImg.value
  if (!img) return
  w = Math.round(Math.max(40, Math.min(w, maxWidth(), img.naturalWidth || 4000)))
  img.style.width = `${w}px`
  img.style.height = 'auto'
  img.style.maxWidth = '100%'
  img.setAttribute('width', String(w))
  img.removeAttribute('height')
  placeFrame()
}
function preset(kind: 'small' | 'fit' | 'orig') {
  const img = selImg.value
  if (!img) return
  if (kind === 'small') setWidth(Math.min(240, img.naturalWidth || 240))
  else if (kind === 'fit') setWidth(Math.min(maxWidth(), img.naturalWidth || maxWidth()))
  else setWidth(img.naturalWidth || maxWidth())
  sync()
}
function removeImage() {
  selImg.value?.remove()
  selImg.value = null
  sync()
}
let drag: { x: number, w: number, dir: number } | null = null
function onHandleDown(e: PointerEvent, dir: number) {
  if (!selImg.value) return
  drag = { x: e.clientX, w: selImg.value.getBoundingClientRect().width, dir }
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  e.preventDefault()
}
function onHandleMove(e: PointerEvent) { if (drag) setWidth(drag.w + (e.clientX - drag.x) * drag.dir) }
function onHandleUp() { if (drag) { drag = null; sync() } }

// ---------- spelling ----------
// Our own check instead of the browser's: wrong words get a red wavy underline (a highlight over the
// text, the HTML stays clean); hovering one or right-clicking it offers the right spellings. The
// signature, the quoted letter, links and code are not checked. The word being typed is left alone
// until the caret leaves it.

const SPELL_SKIP = 'a,code,pre,blockquote,[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd]'
// word: the text under the underline — one word, or two with a misplaced space between them.
type Bad = { range: Range, word: string, sugg: string[] }
let bad: Bad[] = []
let spellT: ReturnType<typeof setTimeout> | undefined
let lastInput = 0
const canPaint = () => typeof CSS !== 'undefined' && 'highlights' in CSS && typeof (window as any).Highlight === 'function'
function sharedHL(): any {
  const reg = (CSS as any).highlights
  let h = reg.get('tm-spell')
  if (!h) { h = new (window as any).Highlight(); reg.set('tm-spell', h) }
  return h
}
function spellSoon(ms: number) { clearTimeout(spellT); spellT = setTimeout(spellPass, ms) }

function textWords() {
  const n = el.value
  const out: { node: Text, start: number, end: number, word: string }[] = []
  if (!n) return out
  const tw = document.createTreeWalker(n, NodeFilter.SHOW_TEXT, {
    acceptNode: tn => (tn.parentElement?.closest(SPELL_SKIP) && n.contains(tn.parentElement!.closest(SPELL_SKIP)) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  })
  for (let tn = tw.nextNode() as Text | null; tn; tn = tw.nextNode() as Text | null) {
    for (const m of tn.data.matchAll(WORD_RE)) {
      // Not a word: part of an address or a link typed without a link (site.ru, name@mail.ru).
      const before = tn.data[m.index! - 1] || '', after = tn.data[m.index! + m[0].length] || ''
      if (/[@./\\_\d]/.test(before) || /[@_\d]/.test(after) || (after === '.' && /\p{L}/u.test(tn.data[m.index! + m[0].length + 1] || ''))) continue
      out.push({ node: tn, start: m.index!, end: m.index! + m[0].length, word: m[0] })
    }
  }
  return out
}
type TW = ReturnType<typeof textWords>[number]
// Two neighbours with one space between them, at least one of them wrong: maybe the space is
// misplaced ("thisf eature").
function suspectPairs(ws: TW[]) {
  const out: { a: TW, b: TW, key: string }[] = []
  for (let i = 0; i + 1 < ws.length; i++) {
    const a = ws[i]!, b = ws[i + 1]!
    if (a.node !== b.node || !/^[ \u00a0]$/.test(a.node.data.slice(a.end, b.start))) continue
    if (isWrong(a.word) || isWrong(b.word)) out.push({ a, b, key: `${a.word} ${b.word}` })
  }
  return out
}
async function spellPass() {
  if (!el.value || !canPaint()) return
  let ws = textWords()
  const todo = ws.filter(w => !isKnown(w.word)).map(w => w.word)
  if (todo.length) await checkWords(todo)
  ws = textWords()
  const ptodo = suspectPairs(ws).filter(p => !isPairKnown(p.key)).map(p => p.key)
  if (ptodo.length) await checkWords([], ptodo)
  paint()
}
function unpaint() {
  if (!canPaint()) return
  const h = sharedHL()
  for (const b of bad) h.delete(b.range)
  bad = []
}
function paint() {
  unpaint()
  if (!el.value || !canPaint()) return
  const h = sharedHL()
  const sel = window.getSelection()
  const caret = sel?.rangeCount && sel.isCollapsed ? sel.getRangeAt(0) : null
  const typing = Date.now() - lastInput < 1500
  const atCaret = (node: Text, start: number, end: number) => typing && caret && caret.startContainer === node && caret.startOffset >= start && caret.startOffset <= end
  const ws = textWords()
  const inPair = new Set<TW>()
  for (const p of suspectPairs(ws)) {
    const fixes = pairFixes(p.key)
    if (!fixes.length || inPair.has(p.a) || atCaret(p.a.node, p.a.start, p.b.end)) continue
    inPair.add(p.a); inPair.add(p.b)
    const r = document.createRange()
    r.setStart(p.a.node, p.a.start)
    r.setEnd(p.b.node, p.b.end)
    // The words one by one may still be what was meant: their own suggestions follow.
    const own = isWrong(p.a.word) && !isWrong(p.b.word) ? suggestions(p.a.word).slice(0, 2).map(s => `${s} ${p.b.word}`) : []
    bad.push({ range: r, word: r.toString(), sugg: [...fixes, ...own].slice(0, 5) })
    h.add(r)
  }
  for (const w of ws) {
    if (inPair.has(w) || !isWrong(w.word) || atCaret(w.node, w.start, w.end)) continue
    const r = document.createRange()
    r.setStart(w.node, w.start)
    r.setEnd(w.node, w.end)
    bad.push({ range: r, word: w.word, sugg: suggestions(w.word).slice(0, 5) })
    h.add(r)
  }
}
function repaint() { paint() }
// While typing: a word that was fixed loses its underline at once, the rest waits for a pause.
function onInputSpell() {
  lastInput = Date.now()
  hideTip()
  if (canPaint()) {
    const h = sharedHL()
    bad = bad.filter(b => { const keep = b.range.toString() === b.word; if (!keep) h.delete(b.range); return keep })
  }
  spellSoon(700)
}
function badAt(x: number, y: number): Bad | null {
  for (const b of bad) {
    for (const r of b.range.getClientRects()) if (x >= r.left - 1 && x <= r.right + 1 && y >= r.top - 2 && y <= r.bottom + 3) return b
  }
  return null
}
function replaceWord(b: Bad, s: string) {
  const n = el.value
  if (!n || b.range.toString() !== b.word) return
  n.focus()
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(b.range.cloneRange())
  document.execCommand('insertText', false, s)
  hideTip(); closeMenu()
  onSelection()
  sync()
  paint()
}
function spellSkip(b: Bad) { for (const w of b.word.split(/[ \u00a0]/)) skipWord(w); hideTip(); closeMenu(); window.dispatchEvent(new Event('tm-spell')) }
async function spellAdd(b: Bad) {
  hideTip(); closeMenu()
  const p = addToDict(b.word)
  window.dispatchEvent(new Event('tm-spell'))
  try { await p; toast(t('spell.added', { word: b.word })) } catch (e: any) { toast(e.message, { kind: 'err' }) }
}

// The hint under a wrong word, after a short hover.
const tip = ref<{ x: number, y: number, b: Bad, sugg: string[] } | null>(null)
const tipEl = ref<HTMLElement | null>(null)
let tipT: ReturnType<typeof setTimeout> | undefined
function onMouseMove(e: MouseEvent) {
  if (menu.value || e.buttons) return
  const b = badAt(e.clientX, e.clientY)
  if (tip.value && b && tip.value.b === b) { clearTimeout(tipT); return }
  clearTimeout(tipT)
  tipT = setTimeout(() => (b ? showTip(b) : hideTip()), b ? 350 : 300)
}
async function showTip(b: Bad) {
  const r = b.range.getClientRects()[0]
  if (!r) return
  tip.value = { x: r.left, y: r.bottom + 4, b, sugg: b.sugg }
  await nextTick()
  const te = tipEl.value
  if (te && tip.value) {
    tip.value.x = Math.max(8, Math.min(tip.value.x, innerWidth - te.offsetWidth - 8))
    if (tip.value.y + te.offsetHeight > innerHeight - 8) tip.value.y = r.top - te.offsetHeight - 4
  }
}
function hideTip() { clearTimeout(tipT); tip.value = null }
function tipEnter() { clearTimeout(tipT) }
function tipLeave() { clearTimeout(tipT); tipT = setTimeout(hideTip, 250) }

// ---------- right-click menu ----------
// Spelling suggestions for a wrong word, formatting, clipboard and (in letters) dictation and Loggi.
// Shift + right click: the browser's own menu.

const menu = ref<{ x: number, y: number, sel: string, spell: Bad | null, sugg: string[] } | null>(null)
const menuEl = ref<HTMLElement | null>(null)
const MENU_COLORS = ['#000000', '#666666', '#cc0000', '#e69138', '#38761d', '#0b5394', '#674ea7', '#ffffff']
const MENU_FONTS = computed(() => [{ label: t('editor.fontSans'), v: 'Arial, Helvetica, sans-serif' }, { label: t('editor.fontSerif'), v: 'Georgia, "Times New Roman", serif' }, { label: t('editor.fontMono'), v: '"Courier New", Courier, monospace' }])
async function onContextMenu(e: MouseEvent) {
  if (e.shiftKey) return
  e.preventDefault()
  onSelection()
  hideTip()
  const b = badAt(e.clientX, e.clientY)
  menu.value = { x: e.clientX, y: e.clientY, sel: selectedText(), spell: b, sugg: b ? b.sugg : [] }
  document.addEventListener('mousedown', menuOutside, true)
  document.addEventListener('keydown', menuKey, true)
  window.addEventListener('scroll', closeMenu, true)
  await nextTick()
  const m = menuEl.value
  if (m && menu.value) { // inside the screen
    menu.value.x = Math.max(8, Math.min(menu.value.x, innerWidth - m.offsetWidth - 8))
    menu.value.y = Math.max(8, Math.min(menu.value.y, innerHeight - m.offsetHeight - 8))
  }
}
function menuOutside(e: MouseEvent) { if (!menuEl.value?.contains(e.target as Node)) closeMenu() }
function menuKey(e: KeyboardEvent) { if (e.key === 'Escape') { e.stopPropagation(); closeMenu() } }
function closeMenu() {
  menu.value = null
  document.removeEventListener('mousedown', menuOutside, true)
  document.removeEventListener('keydown', menuKey, true)
  window.removeEventListener('scroll', closeMenu, true)
}
function menuDo(cmd: string, value?: string) { exec(cmd, value); closeMenu() }
async function menuPaste() {
  closeMenu()
  try {
    const text = await navigator.clipboard.readText()
    if (text) { restore(); document.execCommand('insertText', false, text); sync() }
  } catch { toast(t('editor.clipboardDenied')) }
}
function menuAssist(kind: AssistKind) {
  const text = menu.value?.sel || ''
  closeMenu()
  emit('assist', kind, text)
}
// mark / insertAt: the place of the caret or selection is kept while Loggi or a recording works
// (the user may click elsewhere meanwhile); the text then goes exactly there, replacing the selection.
function mark(): Range | null { return saved && el.value?.contains(saved.startContainer) ? saved.cloneRange() : null }
function insertAt(r: Range | null, text: string) {
  const n = el.value
  if (!n) return
  n.focus()
  const sel = window.getSelection()
  if (r && n.contains(r.startContainer)) { sel?.removeAllRanges(); sel?.addRange(r) } else restore()
  document.execCommand('insertText', false, text)
  onSelection()
  sync()
}

function focusStart() {
  const n = el.value
  if (!n) return
  n.focus()
  const r = document.createRange()
  r.setStart(n, 0)
  r.collapse(true)
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(r)
  n.scrollTop = 0
}
onBeforeUnmount(closeMenu)
// Where the user writes when the caret was never placed: the first empty line of their own text
// (after the greeting of the start template), before the signature and the quoted letter.
const hasCaret = () => !!saved && !!el.value?.contains(saved.startContainer)
function caretToWritingSpot() {
  const n = el.value
  if (!n) return
  const own: Element[] = []
  for (const ch of Array.from(n.children)) {
    if (ch.matches('[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd],blockquote')) break
    own.push(ch)
  }
  const spot = own.find(e => !e.textContent?.trim() && !e.querySelector('img')) || own[own.length - 1]
  const r = document.createRange()
  if (spot) { r.selectNodeContents(spot); r.collapse(!spot.textContent?.trim()) } else { r.setStart(n, 0); r.collapse(true) }
  n.focus()
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(r)
  onSelection()
}
defineExpose({ focus: () => el.value?.focus(), focusStart, exec, insertLink, insertImageFiles, selectedText, mark, insertAt, fmt, hasCaret, caretToWritingSpot })
</script>

<template>
  <div id="c-body" ref="el" class="rte" contenteditable="true" role="textbox" aria-multiline="true" :aria-label="t('editor.bodyLabel')"
       :data-placeholder="props.placeholder || t('editor.bodyLabel')" spellcheck="false"
       @input="sync(); placeFrame(); onInputSpell()" @paste="onPaste" @drop="onDrop" @dragover.prevent @click="onClick" @keydown="onKeydown" @contextmenu="onContextMenu" @mousemove="onMouseMove" @mouseleave="tipLeave" />
  <Teleport to="body">
    <div v-if="menu" ref="menuEl" class="ctx-menu" role="menu" :style="{ left: menu.x + 'px', top: menu.y + 'px' }" @mousedown.prevent @contextmenu.prevent>
      <template v-if="menu.spell">
        <div class="cm-spell-word">{{ /\s/.test(menu.spell.word) ? t('spell.misplacedSpace', { word: menu.spell.word }) : t('spell.maybeWrong', { word: menu.spell.word }) }}</div>
        <button v-for="s in menu.sugg" :key="s" type="button" class="cm-item cm-sugg" @click="replaceWord(menu.spell, s)">{{ s }}</button>
        <p v-if="!menu.sugg.length" class="cm-hint">{{ t('spell.noSuggestions') }}</p>
        <button v-if="!/\s/.test(menu.spell.word)" type="button" class="cm-item" @click="spellAdd(menu.spell)"><AppIcon name="plus" />{{ t('spell.addToDict') }}</button>
        <button type="button" class="cm-item" @click="spellSkip(menu.spell)"><AppIcon name="check" />{{ t('spell.skip') }}</button>
        <hr>
      </template>
      <div class="cm-row">
        <button type="button" :class="{ on: fmt.bold }" :title="t('editor.boldKey')" :aria-label="t('editor.bold')" @click="menuDo('bold')"><AppIcon name="bold" /></button>
        <button type="button" :class="{ on: fmt.italic }" :title="t('editor.italicKey')" :aria-label="t('editor.italic')" @click="menuDo('italic')"><AppIcon name="italic" /></button>
        <button type="button" :class="{ on: fmt.underline }" :title="t('editor.underlineKey')" :aria-label="t('editor.underline')" @click="menuDo('underline')"><AppIcon name="underline" /></button>
        <button type="button" :class="{ on: fmt.strikeThrough }" :title="t('editor.strike')" :aria-label="t('editor.strike')" @click="menuDo('strikeThrough')"><AppIcon name="strike" /></button>
        <i></i>
        <button type="button" :title="t('editor.bulletedList')" :aria-label="t('editor.bulletedList')" @click="menuDo('insertUnorderedList')"><AppIcon name="listUl" /></button>
        <button type="button" :title="t('editor.numberedList')" :aria-label="t('editor.numberedList')" @click="menuDo('insertOrderedList')"><AppIcon name="listOl" /></button>
        <i></i>
        <button type="button" :title="t('editor.removeFormatting')" :aria-label="t('editor.removeFormatting')" @click="menuDo('removeFormat')"><AppIcon name="clearFmt" /></button>
      </div>
      <div class="cm-row cm-colors" :aria-label="t('editor.textColor')">
        <button v-for="c in MENU_COLORS" :key="c" type="button" class="clr" :style="{ background: c }" :title="t('editor.colorValue', { color: c })" @click="menuDo('foreColor', c)"></button>
      </div>
      <div class="cm-row cm-chips">
        <button v-for="f in MENU_FONTS" :key="f.v" type="button" :style="{ fontFamily: f.v }" @click="menuDo('fontName', f.v)">{{ f.label }}</button>
      </div>
      <div class="cm-row cm-chips">
        <button type="button" style="font-size:11px" @click="menuDo('fontSize', '2')">{{ t('editor.sizeSmall') }}</button>
        <button type="button" @click="menuDo('fontSize', '3')">{{ t('editor.sizeNormal') }}</button>
        <button type="button" style="font-size:17px" @click="menuDo('fontSize', '5')">{{ t('editor.sizeLarge') }}</button>
      </div>
      <hr>
      <button type="button" class="cm-item" :disabled="!menu.sel" @click="menuDo('cut')"><AppIcon name="cut" />{{ t('editor.cut') }}<kbd>Ctrl+X</kbd></button>
      <button type="button" class="cm-item" :disabled="!menu.sel" @click="menuDo('copy')"><AppIcon name="copy" />{{ t('common.copy') }}<kbd>Ctrl+C</kbd></button>
      <button type="button" class="cm-item" @click="menuPaste"><AppIcon name="paste" />{{ t('editor.paste') }}<kbd>Ctrl+V</kbd></button>
      <template v-if="props.assist">
        <hr>
        <button type="button" class="cm-item" @click="menuAssist('dictate')"><AppIcon name="mic" />{{ t('voice.dictateText') }}</button>
        <button type="button" class="cm-item ai" @click="menuAssist('dictateLoggi')"><AppIcon name="sparkle" />{{ t('voice.dictateAi', { ai: aiName() }) }}</button>
        <button type="button" class="cm-item ai" :disabled="!menu.sel" :title="menu.sel ? '' : t('ai.selectFirst')" @click="menuAssist('rephrase')"><AppIcon name="sparkle" />{{ t('ai.rephraseSelection') }}</button>
        <button type="button" class="cm-item ai" @click="menuAssist('fix')"><AppIcon name="sparkle" />{{ menu.sel ? t('ai.fixSelection') : t('ai.fixAll') }}</button>
      </template>
      <p class="cm-hint">{{ t('editor.browserMenuHint') }}</p>
    </div>
    <div v-if="tip" ref="tipEl" class="spell-tip" role="menu" :style="{ left: tip.x + 'px', top: tip.y + 'px' }" @mousedown.prevent @mouseenter="tipEnter" @mouseleave="tipLeave">
      <div class="st-sugg">
        <button v-for="(s, i) in tip.sugg" :key="s" type="button" :class="{ first: i === 0 }" @click="replaceWord(tip.b, s)">{{ s }}</button>
        <span v-if="!tip.sugg.length" class="st-none">{{ t('spell.noSuggestions') }}</span>
      </div>
      <div class="st-acts">
        <button v-if="!/\s/.test(tip.b.word)" type="button" @click="spellAdd(tip.b)"><AppIcon name="plus" />{{ t('spell.toDict') }}</button>
        <button type="button" @click="spellSkip(tip.b)"><AppIcon name="check" />{{ t('spell.skip') }}</button>
      </div>
    </div>
    <div v-if="selImg" class="img-frame" :style="frame">
      <span v-for="h in [['nw', -1], ['ne', 1], ['sw', -1], ['se', 1]]" :key="h[0]" class="img-h" :class="h[0]"
            @pointerdown="onHandleDown($event, h[1] as number)" @pointermove="onHandleMove" @pointerup="onHandleUp" @pointercancel="onHandleUp"></span>
      <div class="img-bar" @mousedown.prevent>
        <button type="button" @click="preset('small')">{{ t('editor.imgSmall') }}</button>
        <button type="button" @click="preset('fit')">{{ t('editor.imgFit') }}</button>
        <button type="button" @click="preset('orig')">{{ t('editor.imgOriginal') }}</button>
        <span class="img-size">{{ imgWidth }} px</span>
        <button type="button" class="del" :title="t('editor.deleteImage')" :aria-label="t('editor.deleteImage')" @click="removeImage"><AppIcon name="trash" /></button>
      </div>
    </div>
  </Teleport>
</template>
