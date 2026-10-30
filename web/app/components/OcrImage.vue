<script setup lang="ts">
// A picture with its recognised text laid over it as an invisible, selectable layer (like Google
// Lens in Gmail): drag over the words to select, Ctrl+C to copy.
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { t } from '~/composables/i18n'

export type OcrPage = { w: number, h: number, words: { t: string, x: number, y: number, w: number, h: number, l: number }[] }
const props = defineProps<{ src: string, alt?: string, page?: OcrPage | null, zoom?: number }>()
const natural = ref(0)
const loaded = ref(false) // until the picture has come: the spinner in its place
watch(() => props.src, () => { loaded.value = false })
const img = ref<HTMLImageElement | null>(null)
const scale = ref(0)
let ro: ResizeObserver | null = null
function measure() { if (img.value && props.page?.w) scale.value = img.value.clientWidth / props.page.w }
onMounted(() => { ro = new ResizeObserver(measure); if (img.value) ro.observe(img.value) })
// The text usually arrives after the picture: re-measure then (and when zooming).
watch(() => [props.page, props.zoom], () => nextTick(measure))
onBeforeUnmount(() => ro?.disconnect())

// Words with a line break after the last word of each line, so copied text keeps its lines.
const words = computed(() => {
  const ws = props.page?.words || []
  return ws.map((w, i) => ({ ...w, sep: i === ws.length - 1 ? '' : ws[i + 1].l !== w.l ? '\n' : ' ' }))
})
// Stretch each invisible word to its box, so the selection covers exactly the word on the picture.
let ctx: CanvasRenderingContext2D | null = null
function stretch(w: { t: string, w: number, h: number }) {
  ctx ||= document.createElement('canvas').getContext('2d')
  const size = Math.max(6, w.h * scale.value * 0.92)
  if (!ctx) return 1
  ctx.font = `${size}px Arial, sans-serif`
  const natural = ctx.measureText(w.t).width
  return natural > 0 ? (w.w * scale.value) / natural : 1
}
</script>

<template>
  <div class="ocr-img veil-host" :class="{ waiting: !loaded }">
    <LoadingVeil dark class="compact" :show="!loaded" :text="t('attach.loadingImage')" />
    <img ref="img" :src="props.src" :alt="props.alt" draggable="false" :class="{ zoomed: !!props.zoom }"
         :style="props.zoom && natural ? { width: natural * props.zoom + 'px' } : undefined" @load="natural = img!.naturalWidth; loaded = true; measure()" @error="loaded = true" />
    <div v-if="props.page && scale" class="ocr-layer" :aria-label="t('attach.recognizedText')">
      <span v-for="(w, i) in words" :key="i" :style="{ left: w.x * scale + 'px', top: w.y * scale + 'px', height: w.h * scale + 'px', fontSize: Math.max(6, w.h * scale * 0.92) + 'px', transform: `scaleX(${stretch(w)})` }">{{ w.t + w.sep }}</span>
    </div>
  </div>
</template>

<style scoped>
.ocr-img.waiting{min-width:280px;min-height:200px}
.ocr-img{position:relative;display:inline-block;max-width:100%;max-height:100%;line-height:0}
.ocr-img img{max-width:100%;max-height:calc(100vh - 110px);display:block;border-radius:2px;user-select:none}
.ocr-img img.zoomed{max-width:none;max-height:none}
.ocr-layer{position:absolute;inset:0;line-height:1;cursor:text}
.ocr-layer span{position:absolute;color:transparent;white-space:pre;overflow:visible;transform-origin:0 50%;font-family:Arial,sans-serif;letter-spacing:0;user-select:text}
.ocr-layer span::selection{background:rgba(26,115,232,.38);color:transparent}
</style>
