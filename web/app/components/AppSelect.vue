<script setup lang="ts">
// Themed dropdown replacing the native <select>: button + listbox popover with keyboard support.
// searchable: a filter field on top, for long lists (languages, time zones).
import { ref, computed, nextTick, onBeforeUnmount } from 'vue'

type Opt = { value: string | number, label: string, hint?: string }
const props = defineProps<{ modelValue: string | number, options: Opt[], id?: string, ariaLabel?: string, bare?: boolean, small?: boolean, searchable?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string | number] }>()

const btn = ref<HTMLButtonElement | null>(null)
const list = ref<HTMLUListElement | null>(null)
const open = ref(false)
const active = ref(0)
const pos = ref<Record<string, string>>({})
const uid = `sel-${Math.random().toString(36).slice(2, 8)}`
const current = computed(() => props.options.find(o => o.value === props.modelValue))
const query = ref('')
const queryEl = ref<HTMLInputElement | null>(null)
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? props.options.filter(o => `${o.label} ${o.value} ${o.hint || ''}`.toLowerCase().includes(q)) : props.options
})

function place() {
  const r = btn.value?.getBoundingClientRect()
  if (!r) return
  const width = Math.max(r.width, 200)
  const below = window.innerHeight - r.bottom
  const h = Math.min(320, props.options.length * 38 + 10) + (props.searchable ? 44 : 0)
  const top = below < h + 12 && r.top > below ? r.top - h - 4 : r.bottom + 4
  pos.value = { left: `${Math.max(8, Math.min(r.left, window.innerWidth - width - 8))}px`, top: `${top}px`, minWidth: `${width}px` }
}
async function show() {
  if (open.value) { hide(); return }
  query.value = ''
  active.value = Math.max(0, props.options.findIndex(o => o.value === props.modelValue))
  open.value = true
  place()
  await nextTick()
  queryEl.value?.focus()
  list.value?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
  document.addEventListener('mousedown', outside, true)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', hide)
}
function hide() {
  open.value = false
  document.removeEventListener('mousedown', outside, true)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', hide)
}
const pop = ref<HTMLElement | null>(null)
function onScroll(e: Event) { if (!pop.value?.contains(e.target as Node)) hide() }
function outside(e: MouseEvent) {
  if (!pop.value?.contains(e.target as Node) && !btn.value?.contains(e.target as Node)) hide()
}
function onQuery() { active.value = 0 }
function choose(o: Opt) { emit('update:modelValue', o.value); hide(); btn.value?.focus() }
function onKey(e: KeyboardEvent) {
  const n = shown.value.length
  if (!open.value) {
    if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) { e.preventDefault(); show() }
    return
  }
  if (e.key === 'ArrowDown') { e.preventDefault(); active.value = (active.value + 1) % n }
  else if (e.key === 'ArrowUp') { e.preventDefault(); active.value = (active.value - 1 + n) % n }
  else if (e.key === 'Home') { e.preventDefault(); active.value = 0 }
  else if (e.key === 'End') { e.preventDefault(); active.value = n - 1 }
  else if (e.key === 'Enter' || (e.key === ' ' && !props.searchable)) { e.preventDefault(); const o = shown.value[active.value]; if (o) choose(o) }
  else if (e.key === 'Escape' || e.key === 'Tab') { if (e.key === 'Escape') e.stopPropagation(); hide() }
  nextTick(() => list.value?.children[active.value]?.scrollIntoView({ block: 'nearest' }))
}
onBeforeUnmount(hide)
</script>

<template>
  <button :id="props.id" ref="btn" type="button" class="sel" :class="{ bare: props.bare, sm: props.small, open }"
          aria-haspopup="listbox" :aria-expanded="open" :aria-controls="uid" :aria-label="props.ariaLabel"
          @click="show" @keydown="onKey">
    <span class="sel-val">{{ current?.label ?? '—' }}</span>
    <AppIcon name="chev" />
  </button>
  <Teleport to="body">
    <div v-if="open" ref="pop" class="sel-pop" :style="pos">
    <input v-if="props.searchable" ref="queryEl" v-model="query" class="sel-q" type="text" :aria-label="props.ariaLabel" @input="onQuery" @keydown="onKey" />
    <ul :id="uid" ref="list" class="sel-list" role="listbox" @keydown="onKey">
      <li v-for="(o, i) in shown" :key="String(o.value)" role="option" :aria-selected="o.value === props.modelValue"
          class="sel-opt" :class="{ on: i === active }" @mousedown.prevent @click="choose(o)" @mouseenter="active = i">
        <span>{{ o.label }}<small v-if="o.hint">{{ o.hint }}</small></span>
        <AppIcon v-if="o.value === props.modelValue" name="check" />
      </li>
    </ul>
    </div>
  </Teleport>
</template>
