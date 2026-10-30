<script setup lang="ts">
// Toolbar button with a small popover (font, size, colour, link, emoji, Loggi). The button doesn't
// take focus from the editor, so the selected text stays selected while choosing. The popover is
// placed over the page (fixed, inside the screen): the reply under a letter sits in a container
// that would cut it off.
import { ref, nextTick, onBeforeUnmount } from 'vue'

const props = defineProps<{ title: string, icon?: string, label?: string, down?: boolean, align?: 'left' | 'right' }>()
const emit = defineEmits<{ open: [] }>()
const open = ref(false)
const root = ref<HTMLElement | null>(null)
const pop = ref<HTMLElement | null>(null)
const pos = ref<Record<string, string>>({})
function onDoc(e: MouseEvent) {
  const t = e.target as Node
  if (root.value?.contains(t) || pop.value?.contains(t)) return
  close()
}
function onEsc(e: KeyboardEvent) { if (e.key === 'Escape') { e.stopPropagation(); close() } }
let ro: ResizeObserver | null = null
async function place() {
  const b = root.value?.getBoundingClientRect()
  if (!b) return
  const gap = 6, edge = 8
  // Above the button unless asked for below or there is no room above.
  const p: Record<string, string> = {}
  if (!pop.value) {
    pos.value = { left: '-9999px', top: '0px', visibility: 'hidden' }
    await nextTick()
    // Content that loads after opening (templates) makes it taller: placed again.
    if (pop.value && 'ResizeObserver' in window) { ro = new ResizeObserver(() => place()); ro.observe(pop.value) }
  }
  const w = pop.value?.offsetWidth || 220, h = pop.value?.offsetHeight || 200
  const below = props.down || b.top - gap - h < edge
  p.top = below ? `${Math.min(b.bottom + gap, innerHeight - h - edge)}px` : `${Math.max(edge, b.top - gap - h)}px`
  let left = props.align === 'right' ? b.right - w : b.left
  left = Math.max(edge, Math.min(left, innerWidth - w - edge))
  p.left = `${left}px`
  pos.value = p
}
function toggle() {
  open.value = !open.value
  if (open.value) {
    emit('open')
    place()
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onEsc, true)
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
  } else close()
}
function close() {
  open.value = false
  ro?.disconnect()
  ro = null
  document.removeEventListener('mousedown', onDoc)
  document.removeEventListener('keydown', onEsc, true)
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', place, true)
}
onBeforeUnmount(close)
defineExpose({ close })
</script>

<template>
  <span ref="root" class="tp">
    <button type="button" class="tb-btn" :class="{ on: open, wide: !!props.label }" :title="props.title" :aria-label="props.title" :aria-expanded="open"
            @mousedown.prevent @click="toggle">
      <AppIcon v-if="props.icon" :name="props.icon" /><span v-if="props.label" class="tb-label">{{ props.label }}</span>
      <svg v-if="props.label" class="tb-caret" viewBox="0 0 10 10" aria-hidden="true"><path d="M2 4l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>
    </button>
    <Teleport to="body">
      <div v-if="open" ref="pop" class="tp-pop fixed" :style="pos" @mousedown.self.prevent>
        <slot :close="close" />
      </div>
    </Teleport>
  </span>
</template>
