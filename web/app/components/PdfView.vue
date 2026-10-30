<script setup lang="ts">
// PDF viewer on PDF.js (the engine of Firefox): pages on canvas with the real text layer, so text
// can be selected and copied in every browser. Pages without any text (scans) get recognised text
// (OCR from the server) laid over them instead, automatically.
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import * as pdfjs from 'pdfjs-dist'
import workerSrc from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import 'pdfjs-dist/web/pdf_viewer.css'
import { t } from '~/composables/i18n'

pdfjs.GlobalWorkerOptions.workerSrc = workerSrc

type OcrWord = { t: string, x: number, y: number, w: number, h: number, l: number }
type OcrRes = { pages: { w: number, h: number, words: OcrWord[] }[], text: string }
const props = defineProps<{ src: string, zoom: number, ocrUrl?: string }>()
const emit = defineEmits<{ text: [string], state: ['loading' | 'ready' | 'ocr' | 'error'] }>()
const box = ref<HTMLDivElement | null>(null)
const error = ref('')
const MAX_PAGES = 100
let doc: pdfjs.PDFDocumentProxy | null = null
let gen = 0
let ocr: OcrRes | null = null
let ro: ResizeObserver | null = null
let lastWidth = 0

// Until the first page is drawn the viewer is veiled with the spinner.
const opening = ref(true)
async function load() {
  error.value = ''
  opening.value = true
  emit('state', 'loading')
  try {
    doc = await pdfjs.getDocument({ url: props.src, withCredentials: true, isEvalSupported: false }).promise
    await render()
  } catch (e: any) {
    error.value = t('attach.pdfFailed')
    opening.value = false
    emit('state', 'error')
  }
}

async function render() {
  const d = doc
  const host = box.value
  if (!d || !host) return
  const my = ++gen
  const width = host.clientWidth - 32
  lastWidth = host.clientWidth
  host.querySelectorAll('.pv-page').forEach(n => n.remove())
  let allText = ''
  let anyText = false
  const pages: HTMLDivElement[] = []
  for (let i = 1; i <= Math.min(d.numPages, MAX_PAGES); i++) {
    const page = await d.getPage(i)
    if (my !== gen) return
    const base = page.getViewport({ scale: 1 })
    const scale = props.zoom ? props.zoom * 1.333 : Math.min(width / base.width, 2.5)
    const vp = page.getViewport({ scale })
    const div = document.createElement('div')
    div.className = 'pv-page'
    div.style.width = `${Math.floor(vp.width)}px`
    div.style.height = `${Math.floor(vp.height)}px`
    div.style.setProperty('--scale-factor', String(scale))
    const canvas = document.createElement('canvas')
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.floor(vp.width * dpr)
    canvas.height = Math.floor(vp.height * dpr)
    canvas.style.width = `${Math.floor(vp.width)}px`
    canvas.style.height = `${Math.floor(vp.height)}px`
    div.append(canvas)
    host.append(div)
    pages.push(div)
    await page.render({ canvasContext: canvas.getContext('2d')!, viewport: vp, transform: dpr !== 1 ? [dpr, 0, 0, dpr, 0, 0] : undefined }).promise
    if (my !== gen) return
    opening.value = false
    const content = await page.getTextContent()
    const items = content.items.filter((it: any) => 'str' in it && it.str.trim())
    if (items.length) {
      anyText = true
      allText += items.map((it: any) => it.str + (it.hasEOL ? '\n' : ' ')).join('') + '\n'
      const tl = document.createElement('div')
      tl.className = 'textLayer'
      div.append(tl)
      await new pdfjs.TextLayer({ textContentSource: content, container: tl, viewport: vp }).render()
    }
  }
  if (my !== gen) return
  if (anyText) { emit('text', allText.trim()); emit('state', 'ready'); return }
  // A scan: no text in the file, recognise it on the server.
  if (!props.ocrUrl) { emit('state', 'ready'); return }
  emit('state', 'ocr')
  try {
    ocr ||= await (await fetch(props.ocrUrl, { credentials: 'same-origin', headers: { 'X-ZM': '1' } })).json() as OcrRes
  } catch { ocr = null }
  if (my !== gen) return
  if (ocr) {
    ocr.pages.forEach((p, i) => { if (pages[i]) overlay(pages[i], p) })
    emit('text', ocr.text)
  }
  emit('state', 'ready')
}

// Invisible words over a scanned page, stretched to their boxes (same as OcrImage).
let ctx: CanvasRenderingContext2D | null = null
function overlay(div: HTMLDivElement, p: { w: number, h: number, words: OcrWord[] }) {
  const k = div.clientWidth / p.w
  const layer = document.createElement('div')
  layer.className = 'pv-ocr'
  ctx ||= document.createElement('canvas').getContext('2d')
  p.words.forEach((w, i) => {
    const s = document.createElement('span')
    const size = Math.max(6, w.h * k * 0.92)
    s.textContent = w.t + (i === p.words.length - 1 ? '' : p.words[i + 1].l !== w.l ? '\n' : ' ')
    s.style.cssText = `left:${w.x * k}px;top:${w.y * k}px;height:${w.h * k}px;font-size:${size}px`
    if (ctx) {
      ctx.font = `${size}px Arial, sans-serif`
      const nat = ctx.measureText(w.t).width
      if (nat > 0) s.style.transform = `scaleX(${(w.w * k) / nat})`
    }
    layer.append(s)
  })
  div.append(layer)
}

let timer: ReturnType<typeof setTimeout> | null = null
const rerender = () => { if (timer) clearTimeout(timer); timer = setTimeout(render, 150) }
watch(() => props.zoom, rerender)
watch(() => props.src, () => { ocr = null; doc?.destroy(); load() })
onMounted(async () => {
  await nextTick()
  load()
  ro = new ResizeObserver(() => { if (!props.zoom && box.value && Math.abs(box.value.clientWidth - lastWidth) > 20) rerender() })
  if (box.value) ro.observe(box.value)
})
onBeforeUnmount(() => { gen++; ro?.disconnect(); doc?.destroy() })
</script>

<template>
  <div ref="box" class="pv-doc veil-host">
    <LoadingVeil dark :show="opening && !error" :text="t('attach.openingDoc')" />
    <p v-if="error" class="v-state">{{ error }}</p>
  </div>
</template>

<style>
.pv-doc{width:100%;height:100%;overflow:auto;display:flex;flex-direction:column;align-items:center;gap:12px;padding:0 16px 16px}
.pv-page{position:relative;flex:none;background:#fff;box-shadow:0 2px 12px rgba(0,0,0,.35)}
.pv-page canvas{display:block}
.pv-page .textLayer ::selection{background:rgba(26,115,232,.35)}
.pv-ocr{position:absolute;inset:0;line-height:1;cursor:text}
.pv-ocr span{position:absolute;color:transparent;white-space:pre;transform-origin:0 50%;font-family:Arial,sans-serif}
.pv-ocr span::selection{background:rgba(26,115,232,.38);color:transparent}
</style>
