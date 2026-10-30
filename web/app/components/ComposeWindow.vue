<script setup lang="ts">
// Compose window docked at the bottom right, like Gmail: minimise to a bar, enlarge, close (= keep in Drafts).
// Drag it by the header to move it; double-click the header to dock it back.
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { mail, closeCompose } from '~/composables/mail'
import { t } from '~/composables/i18n'

const c = computed(() => mail.compose!)
const win = ref<HTMLElement | null>(null)
const dragging = ref(false)
const title = computed(() => c.value.subject.trim() || (c.value.mode === 'new' ? t('compose.newLetter') : c.value.mode === 'fwd' ? t('compose.forwarding') : t('compose.reply')))
const toggleMin = () => { mail.composeView = mail.composeView === 'min' ? 'open' : 'min' }
const toggleMax = () => { mail.composeView = mail.composeView === 'max' ? 'open' : 'max' }

const canDrag = () => mail.composeView === 'open' && window.innerWidth > 760
const floating = computed(() => mail.composeView === 'open' && !!mail.composePos)
const style = computed(() => floating.value ? { left: `${mail.composePos!.x}px`, top: `${mail.composePos!.y}px` } : {})

function clamp(x: number, y: number) {
  const r = win.value?.getBoundingClientRect()
  const w = r?.width ?? 600, h = r?.height ?? 600
  return {
    x: Math.round(Math.min(Math.max(8, x), Math.max(8, window.innerWidth - w - 8))),
    y: Math.round(Math.min(Math.max(8, y), Math.max(8, window.innerHeight - h - 8))),
  }
}

let off = { x: 0, y: 0 }
function onDown(e: PointerEvent) {
  if (e.button !== 0 || !canDrag() || (e.target as HTMLElement).closest('button')) return
  const r = win.value!.getBoundingClientRect()
  off = { x: e.clientX - r.left, y: e.clientY - r.top }
  dragging.value = true
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  e.preventDefault()
}
function onMove(e: PointerEvent) {
  if (!dragging.value) return
  mail.composePos = clamp(e.clientX - off.x, e.clientY - off.y)
}
function onUp() { dragging.value = false }
function onHeadClick() { if (mail.composeView === 'min') toggleMin() }
function dock() { if (mail.composeView === 'open') mail.composePos = null }
function onResize() { if (mail.composePos) mail.composePos = clamp(mail.composePos.x, mail.composePos.y) }

onMounted(() => { window.addEventListener('resize', onResize); onResize() })
onBeforeUnmount(() => window.removeEventListener('resize', onResize))
</script>

<template>
  <div v-if="mail.composeView === 'max'" class="cw-scrim" @click="mail.composeView = 'open'"></div>
  <section ref="win" class="cw" :class="[mail.composeView, { floating, dragging }]" :style="style" role="dialog" :aria-label="title">
    <header class="cw-head" :title="mail.composeView === 'open' ? t('compose.dragHint') : ''"
            @pointerdown="onDown" @pointermove="onMove" @pointerup="onUp" @pointercancel="onUp" @click="onHeadClick" @dblclick="dock">
      <b>{{ title }}</b>
      <span v-if="mail.composeView === 'min' && c.saveState === 'saved'" class="cw-saved">{{ t('compose.draftBadge') }}</span>
      <button class="icon-btn" :title="mail.composeView === 'min' ? t('compose.expand') : t('compose.minimize')" :aria-label="mail.composeView === 'min' ? t('compose.expand') : t('compose.minimize')" @click.stop="toggleMin">
        <AppIcon :name="mail.composeView === 'min' ? 'chevUp' : 'minimize'" />
      </button>
      <button v-if="mail.composeView !== 'min'" class="icon-btn" :title="mail.composeView === 'max' ? t('compose.exitFullScreen') : t('compose.fullScreen')" :aria-label="mail.composeView === 'max' ? t('compose.exitFullScreen') : t('compose.fullScreen')" @click.stop="toggleMax">
        <AppIcon :name="mail.composeView === 'max' ? 'shrink' : 'expand'" />
      </button>
      <button class="icon-btn" :title="t('compose.closeKeepsDraft')" :aria-label="t('common.close')" @click.stop="closeCompose()"><AppIcon name="close" /></button>
    </header>
    <div v-show="mail.composeView !== 'min'" class="cw-body"><ComposeForm /></div>
  </section>
</template>
