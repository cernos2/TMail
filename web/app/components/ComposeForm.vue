<script setup lang="ts">
import { computed, ref, onMounted, nextTick, watch } from 'vue'
import { mail, sendCompose, discardDraft, accountOf, switchSender, addInlineImage, addComposeFiles, cancelUpload, removeLink, attachedSize, ATTACH_LIMIT } from '~/composables/mail'
import { fileSize } from '~/composables/format'
import { ai, hasAI, aiName, aiReplyInto, aiWriteInto, aiImproveInto, aiPickTemplate, composeUserText, setComposeUserText, aiRun, letterInput, type AssistKind } from '~/composables/ai'
import { voice, startVoice, stopVoice } from '~/composables/voice'
import { toast } from '~/composables/toast'
import { t, num, fmtDate } from '~/composables/i18n'
import { tpl, loadTemplates, templatesFor, type Template } from '~/composables/templates'

const c = computed(() => mail.compose!)
const senders = computed(() => [
  ...(mail.me?.accounts || []).filter(a => a.canSend).map(a => ({ value: a.address, label: a.name ? `${a.name} <${a.address}>` : a.address })),
  ...mail.identities.filter(i => !mail.me?.accounts.some(a => a.address === i.address)).map(i => ({ value: i.address, label: `${i.name ? `${i.name} <${i.address}>` : i.address}`, hint: t('compose.externalMailbox') })),
])
const fileInput = ref<HTMLInputElement | null>(null)
type Editor = { focus: () => void, focusStart: () => void, exec: (c: string, v?: string) => void, insertLink: (u: string, text: string) => void,
  insertImageFiles: (f: File[]) => void, selectedText: () => string, fmt: Record<string, boolean> }
const body = ref<Editor | null>(null)
const imageInput = ref<HTMLInputElement | null>(null)
const toInput = ref<{ focus: () => void } | null>(null)
const dragging = ref(false)

// The formatting bar stays open or closed the way the user left it.
const fmtOpen = ref(true)
try { fmtOpen.value = localStorage.getItem('tmail.fmtbar') !== '0' } catch {}
function toggleFmt() { fmtOpen.value = !fmtOpen.value; try { localStorage.setItem('tmail.fmtbar', fmtOpen.value ? '1' : '0') } catch {} }

const aiInstr = ref('')
const tplList = computed(() => templatesFor(c.value.from))
const tplPick = ref<{ id: number, busy: boolean } | null>(null)
function tplOpen() { if (!tpl.loaded) loadTemplates(); tplPick.value = null }
const escT = (x: string) => x.replace(/[&<>"]/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[ch]!))
// A template that starts with its own greeting replaces the start template's greeting in a letter
// nobody has written in yet; otherwise it goes where the caret is, or after the greeting.
// Greeting words detected in the first line of a template (detection data, several languages).
const GREETING_RE = /^\s*(здравствуй|добр(ый|ое|ого)|привет|уважаем|hello|hi\b|hey\b|dear\b|good (morning|afternoon|evening)|hallo|guten|sehr geehrte|bonjour|hola|buen[oa]s|buongiorno|olá|bom dia|witam|dzień dobry)/i
function insertTemplate(tp: Template, close: () => void) {
  close()
  const untouched = !hasText.value
  if (untouched && GREETING_RE.test(tp.body)) { setComposeUserText(c.value, tp.body); return }
  if (!body.value?.hasCaret()) body.value?.caretToWritingSpot()
  // The letter already greets: the template's own greeting line would be a second one.
  const text = GREETING_RE.test(tp.body) ? tp.body.replace(/^\s*[^\n]*\n+/, '') : tp.body
  body.value?.exec('insertHTML', text.replace(/\n{3,}/g, '\n\n').trim().split('\n').map(l => `<div>${l.trim() ? escT(l) : '<br>'}</div>`).join(''))
}
async function pickTemplate() {
  tplPick.value = { id: 0, busy: true }
  const id = await aiPickTemplate(tplList.value)
  tplPick.value = { id, busy: false }
}
function manageTemplates(close: () => void) { close(); mail.settingsTab = 'templates' }
const isReply = computed(() => c.value.mode === 'reply' || c.value.mode === 'replyall')
// Something of the user's own beyond the start template: "Write message" can work from it.
const hasText = computed(() => { const own = composeUserText(c.value).replace(/\s+/g, ' ').trim(); return !!own && own !== c.value.template.replace(/\s+/g, ' ').trim() })
function aiDo(kind: 'reply' | 'write' | 'improve', close: () => void) {
  close()
  if (kind === 'reply') aiReplyInto(c.value, aiInstr.value)
  else if (kind === 'write') aiWriteInto(c.value, aiInstr.value)
  else aiImproveInto(c.value, aiInstr.value)
  aiInstr.value = ''
}
const BUSY_KEYS: Record<string, string> = { improve: 'ai.busyImprove', reply: 'ai.busyReply', write: 'ai.busyWrite',
  fix: 'ai.busyFix', rephrase: 'ai.busyRephrase', dictate: 'ai.busyDictate' }
const busyText = (key: string) => t(BUSY_KEYS[key] || 'ai.busyWriting', { ai: aiName() })

// ---------- the editor's right-click menu: dictation and Loggi ----------
const ctxRec = ref<{ kind: AssistKind, at: Range | null } | null>(null)
const ctxRecording = computed(() => voice.owner === 'compose-ctx' && (voice.recording || voice.busy))
const ctxTime = computed(() => `${Math.floor(voice.seconds / 60)}:${String(voice.seconds % 60).padStart(2, '0')}`)
async function onAssist(kind: AssistKind, text: string) {
  const at = body.value?.mark() || null
  if (kind === 'dictate' || kind === 'dictateLoggi') {
    ctxRec.value = { kind, at }
    if (!await startVoice('compose-ctx')) ctxRec.value = null
    return
  }
  if (ai.busy) return
  if (kind === 'fix' && !text) { // the whole letter (without the signature and the quote)
    const all = composeUserText(c.value)
    if (!all.trim()) { toast(t('compose.nothingToFix')); return }
    await loggi('fix', all, x => setComposeUserText(c.value, x))
    return
  }
  await loggi(kind, text, x => body.value?.insertAt(at, x))
}
async function loggi(key: string, draft: string, put: (text: string) => void) {
  ai.busy = key
  try {
    const out = (await aiRun(key, { ...letterInput(), draft, instructions: '' })).trim()
    if (out) put(out)
  } catch (e: any) { toast(e.message, { kind: 'err', ms: 8000 }) } finally { ai.busy = '' }
}
async function ctxStop(cancel = false) {
  const r = ctxRec.value
  const text = (await stopVoice(cancel)).trim()
  ctxRec.value = null
  if (!r || !text || cancel) return
  if (r.kind === 'dictate') body.value?.insertAt(r.at, text + ' ')
  else await loggi('dictate', text, x => body.value?.insertAt(r.at, x + ' '))
}

const link = ref({ url: '', text: '' })
const linkPop = ref<{ close: () => void } | null>(null)
function openLink() { link.value = { url: '', text: body.value?.selectedText() || '' } }
function applyLink(close: () => void) {
  const u = link.value.url.trim()
  if (!u) return
  body.value?.insertLink(u, link.value.text.trim())
  close()
}
const EMOJI = ['😀', '😊', '🙂', '😉', '😍', '🤝', '👍', '👌', '🙏', '👏', '💪', '🎉', '✅', '❌', '⚠️', '❗', '❓', '📌', '📎', '📄',
  '📦', '🚚', '💰', '💳', '📅', '⏰', '📞', '✉️', '🔥', '⭐', '❤️', '😅', '🤔', '😢', '😮', '🙌', '➡️', '⬆️', '🔧', '🏭']
function addImages(list: FileList | null) {
  if (list?.length) body.value?.insertImageFiles(Array.from(list))
  if (imageInput.value) imageInput.value.value = ''
}
// Files pasted outside the text (into Subject, To…) are attached; text pastes behave normally.
function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files || [])
  if (!files.length) return
  e.preventDefault()
  addFiles(files)
}

// Explain why the reply goes out from this address (or why it cannot).
const fromNote = computed(() => {
  const src = c.value.sourceAccount
  if (!src || c.value.mode === 'new') return ''
  if (c.value.from === src) return t('compose.fromNoteSame', { address: src })
  if (mail.identities.some(i => i.address === c.value.from && i.mailbox === src)) return t('compose.fromNoteIdentity', { address: c.value.from })
  return accountOf(src)?.canSend === false ? t('compose.fromNoteReadOnly', { source: src, from: c.value.from }) : ''
})

const saveNote = computed(() => {
  const x = c.value
  if (x.saveState === 'saving') return t('compose.savingDraft')
  if (x.saveState === 'error') return t('compose.draftNotSaved')
  if (x.saveState === 'saved' && x.savedAt) return t('compose.draftSavedAt', { time: fmtDate(x.savedAt, { hour: '2-digit', minute: '2-digit' }) })
  return ''
})

watch(() => mail.compose?.from, (from, prev) => { if (from && prev && from !== prev && mail.compose) switchSender(mail.compose, from) })

function addFiles(list: FileList | File[] | null) {
  if (!list) return
  addComposeFiles(c.value, Array.from(list))
  if (fileInput.value) fileInput.value.value = ''
}
const pct = (u: { loaded: number, size: number }) => Math.min(100, Math.round(u.loaded / Math.max(1, u.size) * 100))
const mb = (n: number) => { const d = n > 10485760 ? 0 : 1; return num(n / 1048576, { minimumFractionDigits: d, maximumFractionDigits: d }) }
function removeCarry(i: number) {
  c.value.carry!.parts.splice(i, 1)
  if (!c.value.carry!.parts.length) c.value.carry = null
}
// Drops outside the text area attach files; drops onto the text are handled by the editor.
function onDrop(e: DragEvent) { dragging.value = false; addFiles(e.dataTransfer?.files || null) }
function onKey(e: KeyboardEvent) { if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); sendCompose() } }

onMounted(async () => {
  await nextTick()
  if ((c.value.mode === 'new' && !c.value.to) || c.value.mode === 'fwd') toInput.value?.focus()
  else body.value?.focusStart()
})
</script>

<template>
  <div class="composer veil-host" :class="{ drag: dragging }" @dragover.prevent="dragging = true" @dragleave="dragging = false" @drop.prevent="onDrop" @keydown="onKey" @paste="onPaste">
    <LoadingVeil :show="!!c.sending" :text="t('compose.sendingLetter')" />
    <div class="fld">
      <label for="c-from">{{ t('compose.from') }}</label>
      <AppSelect id="c-from" v-model="c.from" :options="senders" bare :aria-label="t('compose.senderAddress')" />
    </div>
    <div v-if="fromNote" class="from-note"><AppIcon name="info" />{{ fromNote }}</div>
    <div class="fld">
      <label for="c-to">{{ t('compose.to') }}</label>
      <AddressInput id="c-to" ref="toInput" v-model="c.to" :placeholder="t('compose.toPlaceholder')" />
      <button v-if="!c.showCc" class="cc-toggle" type="button" @click="c.showCc = true">{{ t('compose.cc') }}</button>
    </div>
    <template v-if="c.showCc">
      <div class="fld"><label for="c-cc">{{ t('compose.cc') }}</label><AddressInput id="c-cc" v-model="c.cc" /></div>
      <div class="fld"><label for="c-bcc">{{ t('compose.bcc') }}</label><AddressInput id="c-bcc" v-model="c.bcc" :placeholder="t('compose.bccPlaceholder')" /></div>
    </template>
    <div class="fld"><label for="c-subj">{{ t('compose.subject') }}</label><input id="c-subj" v-model="c.subject" type="text" /></div>
    <div class="veil-host editor-host">
      <LoadingVeil class="compact" :show="!!ai.busy" :text="busyText(ai.busy)" />
      <RichEditor ref="body" v-model="c.html" :add-image="addInlineImage" assist @assist="onAssist" :placeholder="t('compose.bodyPlaceholder')"
                  @files="fs => addFiles(fs)" />
    </div>
    <!-- Error, formatting and "Send": one block that stays at the bottom of the reading pane. -->
    <Teleport to="#reader-dock" defer :disabled="mail.composeModal">
    <div class="c-dock">
    <div v-if="c.uploads.length || c.links.length" class="files big">
      <span v-for="u in c.uploads" :key="u.id" class="file-chip up" :class="{ err: u.error }" :title="u.error || t('compose.bigFileTitle')">
        <AppIcon name="download" class="up-ico" /><span>{{ u.name }}</span>
        <small class="hint">{{ u.error ? u.error : t('compose.uploadProgress', { pct: num(pct(u)), loaded: mb(u.loaded), size: mb(u.size) }) }}</small>
        <i v-if="!u.error" class="up-bar"><b :style="{ width: pct(u) + '%' }"></b></i>
        <button type="button" :aria-label="t('compose.cancelUpload', { name: u.name })" @click="cancelUpload(c, u)"><AppIcon name="close" /></button></span>
      <span v-for="l in c.links" :key="l.token" class="file-chip link" :title="t('compose.sentAsLink', { url: l.url })">
        <AppIcon name="link" /><span>{{ l.name }}</span><small class="hint">{{ t('compose.linkSize', { size: mb(l.size) }) }}</small>
        <button type="button" :aria-label="t('compose.removeFile', { name: l.name })" @click="removeLink(c, l.token)"><AppIcon name="close" /></button></span>
    </div>
    <div v-if="c.files.length || c.carry?.parts.length" class="files">
      <span v-for="(p, i) in c.carry?.parts || []" :key="'c' + p.part" class="file-chip" :title="c.mode === 'fwd' && !c.draftUid ? t('compose.fwdAttachment') : t('compose.savedInDraft')">
        <AppIcon name="clip" /><span>{{ p.name }}</span><small class="hint">{{ fileSize(p.size) }}</small>
        <button type="button" :aria-label="t('compose.removeFile', { name: p.name })" @click="removeCarry(i)"><AppIcon name="close" /></button></span>
      <span v-for="(f, i) in c.files" :key="i" class="file-chip" :class="{ uploading: c.saveState === 'saving' }"><AppSpinner v-if="c.saveState === 'saving'" size="sm" /><AppIcon v-else name="file" /><span>{{ f.name }}</span><small class="hint">{{ c.saveState === 'saving' ? t('compose.fileUploading') : c.saveState === 'error' ? t('compose.fileUploadFailed') : fileSize(f.size) }}</small>
        <button type="button" :aria-label="t('compose.removeFile', { name: f.name })" @click="c.files.splice(i, 1)"><AppIcon name="close" /></button></span>
    </div>
    <p v-if="c.error" class="err" role="alert">{{ c.error }}</p>
    <EditorToolbar v-if="fmtOpen" :ed="body" />
    <div class="foot">
      <button class="btn primary" :disabled="c.sending" @click="sendCompose"><AppSpinner v-if="c.sending" size="sm" light /><AppIcon v-else name="sent" />{{ c.sending ? t('compose.sending') : t('common.send') }}</button>
      <!-- Dictation: the recognised text goes where the cursor is in the letter. -->
      <VoiceButton v-if="!ctxRecording" id="compose-body" small :label="t('voice.byVoice')" @text="txt => body?.exec('insertText', txt + ' ')" />
      <!-- A recording started from the right-click menu: stop it here. -->
      <span v-else class="ctx-rec" role="status">
        <span class="v-dot"></span>{{ ctxRec?.kind === 'dictateLoggi'
          ? (voice.busy ? t('voice.recognizingAi', { ai: aiName() }) : t('voice.recordingTimeAi', { time: ctxTime, ai: aiName() }))
          : (voice.busy ? t('voice.recognizing') : t('voice.recordingTime', { time: ctxTime })) }}
        <button type="button" class="btn sm primary" :disabled="voice.busy" @mousedown.prevent @click="ctxStop()">{{ t('common.done') }}</button>
        <button type="button" class="tb-btn" :title="t('voice.cancelRecording')" :aria-label="t('voice.cancelRecording')" :disabled="voice.busy" @mousedown.prevent @click="ctxStop(true)"><AppIcon name="close" /></button>
      </span>
      <span class="tools">
        <button type="button" class="tb-btn" :class="{ on: fmtOpen }" :title="t('compose.formatting')" :aria-label="t('compose.formatting')" :aria-pressed="fmtOpen" @mousedown.prevent @click="toggleFmt"><AppIcon name="textAa" /></button>
        <button type="button" class="tb-btn" :title="t('compose.attachFiles')" :aria-label="t('compose.attachFiles')" @click="fileInput?.click()"><AppIcon name="clip" /></button>
        <ToolPop ref="linkPop" v-slot="{ close }" :title="t('editor.insertLink')" icon="link" @open="openLink">
          <form class="link-form" @submit.prevent="applyLink(close)">
            <label>{{ t('editor.text') }}<input v-model="link.text" type="text" :placeholder="t('compose.linkTextPlaceholder')" /></label>
            <label>{{ t('compose.linkUrl') }}<input v-model="link.url" type="text" :placeholder="t('compose.linkUrlPlaceholder')" autofocus /></label>
            <div class="link-btns"><button type="button" class="btn sm" @click="close()">{{ t('common.cancel') }}</button><button class="btn sm primary" :disabled="!link.url.trim()">{{ t('editor.insert') }}</button></div>
          </form>
        </ToolPop>
        <ToolPop v-slot="{ close }" :title="t('compose.emoji')" icon="smile">
          <div class="emoji-grid"><button v-for="e in EMOJI" :key="e" type="button" @mousedown.prevent @click="body?.exec('insertText', e); close()">{{ e }}</button></div>
        </ToolPop>
        <ToolPop v-slot="{ close }" :title="t('compose.templates')" :label="t('compose.templatesShort')" icon="list" @open="tplOpen">
          <div class="tpl-pop">
            <button v-if="isReply && hasAI('pick_template') && tplList.length" type="button" class="btn sm ai" :disabled="tplPick?.busy" @click="pickTemplate">
              <AppSpinner v-if="tplPick?.busy" size="sm" /><AppIcon v-else name="sparkle" />{{ tplPick?.busy ? t('ai.readingLetter', { ai: aiName() }) : t('compose.pickTemplate') }}</button>
            <p v-if="tplPick && !tplPick.busy && !tplPick.id" class="hint">{{ t('compose.noTemplateFound', { ai: aiName() }) }}</p>
            <div class="move-list">
              <button v-for="tp in tplList" :key="tp.id" type="button" class="tp-item tpl-row" :class="{ best: tplPick?.id === tp.id }" :title="tp.body" @mousedown.prevent @click="insertTemplate(tp, close)">
                <b>{{ tp.title }}<span v-if="tplPick?.id === tp.id" class="tpl-best">{{ t('compose.templateFits') }}</span></b><span>{{ tp.body.replace(/\s+/g, ' ').slice(0, 70) }}</span>
              </button>
              <p v-if="tpl.loaded && !tplList.length" class="hint" style="padding:6px 10px">{{ t('compose.noTemplates') }}</p>
            </div>
            <button type="button" class="link" style="align-self:flex-start;padding:4px 8px" @click="manageTemplates(close)">{{ t('compose.manageTemplates') }}</button>
          </div>
        </ToolPop>
        <ToolPop v-if="ai.actions.length" v-slot="{ close }" :title="t('ai.assistantTitle', { ai: aiName() })" icon="sparkle" :label="aiName()">
          <div class="ai-pop">
            <b>{{ aiName() }}</b>
            <textarea v-model="aiInstr" :placeholder="isReply ? t('ai.replyPlaceholder') : t('ai.writePlaceholder')"></textarea>
            <div class="row-btns">
              <VoiceButton id="compose" small @text="txt => aiInstr = (aiInstr ? aiInstr + ' ' : '') + txt" />
              <span class="grow"></span>
              <button v-if="isReply && hasAI('reply')" type="button" class="btn sm primary" :disabled="!!ai.busy" @click="aiDo('reply', close)">{{ t('ai.writeReply') }}</button>
              <button v-if="!isReply && hasAI('write')" type="button" class="btn sm primary" :disabled="!!ai.busy || !aiInstr.trim() && !hasText" @click="aiDo('write', close)">{{ t('ai.writeLetter') }}</button>
              <button v-if="hasAI('improve')" type="button" class="btn sm" :disabled="!!ai.busy" @click="aiDo('improve', close)">{{ t('ai.improveText') }}</button>
            </div>
            <span class="hint">{{ isReply ? t('ai.replyHint', { ai: aiName() }) : t('ai.writeHint', { ai: aiName() }) }}</span>
          </div>
        </ToolPop>
        <button type="button" class="tb-btn" :title="t('compose.insertImage')" :aria-label="t('compose.insertImage')" @mousedown.prevent @click="imageInput?.click()"><AppIcon name="image" /></button>
      </span>
      <input ref="fileInput" type="file" multiple hidden @change="addFiles(($event.target as HTMLInputElement).files)" />
      <input ref="imageInput" type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden @change="addImages(($event.target as HTMLInputElement).files)" />
      <span v-if="c.uploads.some(u => !u.error)" class="hint save-note">{{ t('compose.uploadingLarge') }}</span>
      <span v-else class="hint save-note" :class="{ err: c.saveState === 'error' }" aria-live="polite">{{ ai.busy ? t('ai.busyWriting', { ai: aiName() }) : saveNote || t('compose.ctrlEnterHint') }}</span>
      <span class="grow"></span>
      <button class="icon-btn" type="button" :title="t('compose.discardDraft')" :aria-label="t('compose.discardDraft')" @click="discardDraft()"><AppIcon name="trash" /></button>
    </div>
    </div>
    </Teleport>
  </div>
</template>
