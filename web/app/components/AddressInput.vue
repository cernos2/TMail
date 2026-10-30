<script setup lang="ts">
// Recipient field with suggestions from the address book (people we wrote to or who wrote to us).
// Works on the last comma-separated entry; choosing a suggestion replaces it with "Name <email>, ".
import { ref, computed, nextTick, onBeforeUnmount } from 'vue'
import { api } from '~/composables/api'
import { t, num } from '~/composables/i18n'

type Hit = { email: string, name: string, sent: number, recv: number, company?: string, position?: string }
const props = defineProps<{ modelValue: string, id: string, placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const input = ref<HTMLInputElement | null>(null)
const hits = ref<Hit[]>([])
const active = ref(0)
const open = ref(false)
const pos = ref<Record<string, string>>({})
// When several people of one company match, offer adding all of them at once.
const group = ref<{ company: string, hits: Hit[] } | null>(null)
function groupFor(q: string, list: Hit[]) {
  const by: Record<string, Hit[]> = {}
  for (const h of list) if (h.company) (by[h.company] ||= []).push(h)
  const best = Object.entries(by).find(([c, hs]) => hs.length > 1 && c.toLowerCase().includes(q.toLowerCase()))
  return best ? { company: best[0], hits: best[1] } : null
}
async function pickAll(hs: Hit[]) {
  const v = props.modelValue
  const head = v.slice(0, v.lastIndexOf(',') + 1)
  const fmt = (h: Hit) => h.name && !/[",<>]/.test(h.name) ? `${h.name} <${h.email}>` : h.email
  emit('update:modelValue', `${head ? head.trimEnd() + ' ' : ''}${hs.map(fmt).join(', ')}, `)
  close()
  await nextTick()
  input.value?.focus()
}
let timer: ReturnType<typeof setTimeout> | null = null
let seq = 0

const listId = computed(() => props.id + '-suggest')
const token = (v: string) => v.slice(v.lastIndexOf(',') + 1).trim()
const already = computed(() => new Set((props.modelValue.match(/[^\s<>,;"]+@[^\s<>,;"]+/g) || []).map(e => e.toLowerCase())))

function place() {
  const r = input.value?.getBoundingClientRect()
  if (!r) return
  const width = Math.min(Math.max(r.width, 320), window.innerWidth - 24)
  pos.value = { left: `${Math.max(12, Math.min(r.left, window.innerWidth - width - 12))}px`, top: `${r.bottom + 4}px`, width: `${width}px` }
}

function onInput(e: Event) {
  const v = (e.target as HTMLInputElement).value
  emit('update:modelValue', v)
  const q = token(v)
  if (timer) clearTimeout(timer)
  if (!q) { close(); return }
  timer = setTimeout(async () => {
    const my = ++seq
    try {
      const res = await api<Hit[]>('/api/contacts', { query: { q } })
      if (my !== seq) return
      hits.value = res.filter(h => !already.value.has(h.email) || token(props.modelValue).toLowerCase() === h.email)
      group.value = groupFor(q, hits.value)
      active.value = 0
      open.value = hits.value.length > 0
      if (open.value) { place(); window.addEventListener('scroll', place, true); window.addEventListener('resize', place) }
    } catch { close() }
  }, 160)
}

function close() {
  group.value = null
  seq++ // drop answers to searches still in flight
  if (timer) { clearTimeout(timer); timer = null }
  open.value = false
  hits.value = []
  window.removeEventListener('scroll', place, true)
  window.removeEventListener('resize', place)
}

async function pick(h: Hit) {
  const v = props.modelValue
  const head = v.slice(0, v.lastIndexOf(',') + 1)
  const name = h.name && !/[",<>]/.test(h.name) ? h.name : h.name ? `"${h.name.replace(/"/g, '')}"` : ''
  emit('update:modelValue', `${head ? head.trimEnd() + ' ' : ''}${name ? `${name} <${h.email}>` : h.email}, `)
  close()
  await nextTick()
  input.value?.focus()
}

function onKey(e: KeyboardEvent) {
  if (!open.value) return
  if (e.key === 'ArrowDown') { e.preventDefault(); active.value = (active.value + 1) % hits.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); active.value = (active.value - 1 + hits.value.length) % hits.value.length }
  else if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pick(hits.value[active.value]) }
  else if (e.key === 'Escape') { e.stopPropagation(); close() }
}

// Delay so a click on a suggestion lands before the list disappears.
function onBlur() { setTimeout(close, 150) }
const note = (h: Hit) => h.sent ? t('compose.youWrote', { n: num(h.sent) }) : h.recv ? t('compose.wroteYou', { n: num(h.recv) }) : ''
// The company line under the name, so "Service" shows who works at "Service DM".
const sub = (h: Hit) => [h.name ? h.email : '', [h.position, h.company].filter(Boolean).join(' · ')].filter(Boolean).join(' — ')
defineExpose({ focus: () => input.value?.focus() })
onBeforeUnmount(close)
</script>

<template>
  <input :id="props.id" ref="input" :value="props.modelValue" type="text" autocomplete="off" :placeholder="props.placeholder"
         role="combobox" :aria-expanded="open" :aria-controls="listId" aria-autocomplete="list"
         :aria-activedescendant="open ? `${listId}-${active}` : undefined"
         @input="onInput" @keydown="onKey" @blur="onBlur" />
  <Teleport to="body">
    <ul v-if="open" :id="listId" class="ac-list" role="listbox" :style="pos">
      <li v-if="group" class="ac-item ac-group" role="option" :aria-selected="false" @mousedown.prevent @click="pickAll(group.hits)">
        <span class="ac-ava ac-co"><AppIcon name="users" /></span>
        <span class="ac-txt"><b>{{ t('compose.allContactsOf', { company: group.company }) }}</b><span>{{ t('compose.groupPeople', { n: group.hits.length, names: group.hits.map(h => h.name || h.email).join(', ') }) }}</span></span>
      </li>
      <li v-for="(h, i) in hits" :id="`${listId}-${i}`" :key="h.email" role="option" :aria-selected="i === active"
          class="ac-item" :class="{ on: i === active }" @mousedown.prevent @click="pick(h)" @mouseenter="active = i">
        <AppAvatar class="ac-ava" :email="h.email" :name="h.name || h.email" />
        <span class="ac-txt"><b>{{ h.name || h.email }}</b><span v-if="sub(h)">{{ sub(h) }}</span></span>
        <span class="ac-note">{{ note(h) }}</span>
      </li>
    </ul>
  </Teleport>
</template>
