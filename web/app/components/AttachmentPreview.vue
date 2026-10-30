<script setup lang="ts">
// Full-screen file viewer like Gmail: images, PDFs and documents as pages, spreadsheets as a grid
// with selectable cells and sheet tabs, text files as text. Arrows go to the next/previous file of
// any letter in the list.
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { pv, attUrl, closePreview, stepPreview, current, ext, extColor, canPreview, pageUrl, zoomBy, zoomFit, zoomActual, guardDownload } from '~/composables/attachments'
import { copyText } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { fileSize } from '~/composables/format'
import { t, num } from '~/composables/i18n'

const f = computed(() => current())
const a = computed(() => f.value?.att)
// Text of the open PDF/document (its own text layer or OCR), for "copy all".
const docText = ref('')
const docState = ref('')
watch(() => pv.index, () => { docText.value = ''; docState.value = '' })
async function copyAll() { const text = pv.ocr?.text || docText.value; if (text && await copyText(text)) toast(t('attach.textCopied')) }
const isPdf = computed(() => a.value?.preview === 'pdf')
const zoomable = computed(() => !!a.value && !pv.loading && !pv.error && (!!a.value.preview || a.value.mime.startsWith('image/')))
const zoomLabel = computed(() => pv.zoom ? num(Math.round(pv.zoom * 100) / 100, { style: 'percent' }) : t('attach.zoomFit'))
// PDF viewer zoom goes through the URL (#zoom= / #view=FitH); the key reloads it.
const sheetDoc = computed(() => { const d = pv.sheets?.[pv.sheet]?.doc || ''; return pv.zoom ? d.replace('</head>', `<style>html{zoom:${pv.zoom}}</style></head>`) : d })
const textLines = computed(() => (pv.text ?? '').replace(/\r\n?/g, '\n').split('\n'))
function onWheel(e: WheelEvent) {
  if (!e.ctrlKey || !zoomable.value) return
  e.preventDefault()
  zoomBy(e.deltaY < 0 ? 1 : -1)
}
const pos = computed(() => pv.items.length > 1 ? t('attach.position', { i: pv.index + 1, n: pv.items.length }) : '')
function onKey(e: KeyboardEvent) {
  if (!f.value) return
  if (e.key === 'Escape') { e.stopPropagation(); closePreview() }
  else if (e.key === 'ArrowRight' && !(e.target as HTMLElement).closest?.('pre,.np')) stepPreview(1)
  else if (e.key === 'ArrowLeft' && !(e.target as HTMLElement).closest?.('pre,.np')) stepPreview(-1)
  else if ((e.ctrlKey || e.metaKey) && (e.key === '=' || e.key === '+')) { e.preventDefault(); zoomBy(1) }
  else if ((e.ctrlKey || e.metaKey) && e.key === '-') { e.preventDefault(); zoomBy(-1) }
  else if ((e.ctrlKey || e.metaKey) && e.key === '0') { e.preventDefault(); zoomFit() }
}
onMounted(() => window.addEventListener('keydown', onKey, true))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
// A click on the dark area around the document (not on its pages, text, sheet or buttons) closes it.
function onBackdrop(e: MouseEvent) {
  const el = e.target as HTMLElement
  if (window.getSelection()?.toString()) return // the end of selecting text
  if (el.closest('.pv-page, .ocr-img, .v-sheet, .v-tabs, .np, .v-state, .v-nav, a, button, iframe, img, canvas')) return
  closePreview()
}
</script>

<template>
  <Teleport to="body">
    <div v-if="f && a" class="viewer" role="dialog" aria-modal="true" :aria-label="a.name">
      <header class="viewer-head">
        <button class="v-btn" :title="t('attach.closeKey')" :aria-label="t('common.close')" @click="closePreview"><AppIcon name="close" /></button>
        <span class="v-ext" :style="{ background: extColor(a) }">{{ ext(a) || 'FILE' }}</span>
        <div class="v-title"><b>{{ a.name }}</b><span>{{ fileSize(a.size) }}<template v-if="pos"> · {{ pos }}</template></span></div>
        <span v-if="zoomable" class="v-zoom" role="group" :aria-label="t('attach.zoom')">
          <button class="v-btn" :title="t('attach.zoomOutKey')" :aria-label="t('attach.zoomOut')" @click="zoomBy(-1)"><AppIcon name="minimize" /></button>
          <button class="v-zoom-val" :title="t('attach.zoomToggle')" @click="pv.zoom ? zoomFit() : zoomActual()">{{ zoomLabel }}</button>
          <button class="v-btn" :title="t('attach.zoomInKey')" :aria-label="t('attach.zoomIn')" @click="zoomBy(1)"><AppIcon name="plus" /></button>
        </span>
        <span v-if="pv.ocrBusy || docState === 'ocr'" class="v-ocr-state"><AppSpinner size="sm" light />{{ t('attach.recognizing') }}</span>
        <button v-else-if="pv.ocr?.text || docText" class="v-btn wide" :title="t('attach.copyAllHint')" @click="copyAll"><AppIcon name="copy" /><span>{{ t('attach.copyAll') }}</span></button>
        <a class="v-btn wide" :href="attUrl(f.msg, a, true)" @click="guardDownload($event, f.msg, a)" :title="t('common.download')"><AppIcon name="download" /><span>{{ t('common.download') }}</span></a>
      </header>
      <div class="viewer-body" @click="onBackdrop" @wheel="onWheel">
        <LoadingVeil v-if="pv.loading" dark show :text="pv.checking ? t('attach.scanningOpen') : a.preview === 'office' || a.preview === 'sheet' ? t('attach.preparingDoc') : t('attach.loadingFile')" />
        <div v-else-if="pv.error" class="v-state"><p>{{ pv.error }}</p><a v-if="!pv.blocked" class="btn" :href="attUrl(f.msg, a, true)" @click="guardDownload($event, f.msg, a)"><AppIcon name="download" />{{ t('attach.downloadFile') }}</a></div>
        <template v-else-if="pv.sheets">
          <div class="v-sheet">
            <iframe :key="pv.index + ':' + pv.sheet + ':' + pv.zoom" sandbox="" :srcdoc="sheetDoc" :title="a.name" />
          </div>
          <nav v-if="pv.sheets.length > 1" class="v-tabs" :aria-label="t('attach.sheets')">
            <button v-for="(s, i) in pv.sheets" :key="i" type="button" :class="{ on: i === pv.sheet }" @click="pv.sheet = i">{{ s.name }}</button>
          </nav>
        </template>
        <div v-else-if="pv.text !== undefined" class="np" :style="{ fontSize: 13 * (pv.zoom || 1) + 'px' }">
          <div class="np-gutter" aria-hidden="true"><span v-for="(_, i) in textLines" :key="i">{{ i + 1 }}</span></div>
          <pre class="np-code">{{ pv.text }}</pre>
        </div>
        <PdfView v-else-if="pv.url" :key="'o' + pv.index" :src="pv.url" :zoom="pv.zoom" @text="text => docText = text" @state="s => docState = s" />
        <div v-else-if="pv.scan" class="v-pages" :style="{ '--pw': pv.zoom ? 1000 * pv.zoom + 'px' : '100%' }">
          <p v-if="pv.ocr && !pv.ocr.text" class="v-state">{{ t('attach.noTextFound') }}</p>
          <OcrImage v-for="(pg, i) in pv.ocr?.pages || []" :key="i" :src="pageUrl(f, i + 1)" :page="pg" :alt="t('attach.pageAlt', { name: a.name, n: i + 1 })" />
          <div v-if="pv.ocrBusy" class="v-state"><AppSpinner size="lg" light />{{ t('attach.recognizingPages') }}</div>
        </div>
        <div v-else-if="a.mime.startsWith('image/')" class="v-imgwrap" :class="{ fit: !pv.zoom }">
          <OcrImage :key="pv.index" :src="attUrl(f.msg, a)" :alt="a.name" :page="pv.ocr?.pages[0]" :zoom="pv.zoom" />
        </div>
        <PdfView v-else-if="isPdf" :key="'p' + pv.index" :src="attUrl(f.msg, a)" :ocr-url="attUrl(f.msg, a, false, 'ocr')" :zoom="pv.zoom" @text="text => docText = text" @state="s => docState = s" />
        <iframe v-else-if="canPreview(a) && !pv.scan" class="v-pdf" :src="attUrl(f.msg, a)" :title="a.name" />
        <div v-else class="v-state"><AppIcon name="file" /><p>{{ t('attach.noPreview') }}</p><a class="btn primary" :href="attUrl(f.msg, a, true)" @click="guardDownload($event, f.msg, a)"><AppIcon name="download" />{{ t('common.download') }}</a></div>
        <template v-if="pv.items.length > 1">
          <button class="v-nav prev" :title="t('attach.prevFileKey')" :aria-label="t('attach.prevFile')" @click="stepPreview(-1)"><AppIcon name="back" /></button>
          <button class="v-nav next" :title="t('attach.nextFileKey')" :aria-label="t('attach.nextFile')" @click="stepPreview(1)"><AppIcon name="back" /></button>
        </template>
      </div>
    </div>
  </Teleport>
</template>
