<script setup lang="ts">
// A date field with a month calendar: the value is "YYYY-MM-DD" (like <input type="date">), shown
// and typed as DD.MM.YYYY. The calendar opens from the field or its icon; weeks start on Monday.
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { t, monthNames, weekdayNames } from '~/composables/i18n'

const model = defineModel<string>({ default: '' })
const props = defineProps<{ min?: string, max?: string, required?: boolean, ariaLabel?: string }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const pad = (n: number) => String(n).padStart(2, '0')
const iso = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
const parseISO = (s?: string) => { const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || ''); return m ? new Date(+m[1]!, +m[2]! - 1, +m[3]!) : null }
const show = (s: string) => { const d = parseISO(s); return d ? `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}` : '' }

const text = ref(show(model.value))
watch(model, v => { text.value = show(v) })
// Typed by hand: 26.10.2026, 26.10.26, 26/10/2026.
function commit() {
  const m = /^\s*(\d{1,2})[./-](\d{1,2})[./-](\d{2,4})\s*$/.exec(text.value)
  if (!m) { if (!text.value.trim() && !props.required) model.value = ''; else text.value = show(model.value); return }
  const y = m[3]!.length === 2 ? 2000 + +m[3]! : +m[3]!
  const d = new Date(y, +m[2]! - 1, +m[1]!)
  if (d.getMonth() !== +m[2]! - 1 || !allowed(d)) { text.value = show(model.value); return }
  model.value = iso(d)
}

const month = ref(new Date())
function toggle() {
  open.value = !open.value
  if (open.value) { const d = parseISO(model.value) || new Date(); month.value = new Date(d.getFullYear(), d.getMonth(), 1) }
}
function shift(n: number) { month.value = new Date(month.value.getFullYear(), month.value.getMonth() + n, 1) }
const title = computed(() => t('calendar.monthYear', { month: monthNames()[month.value.getMonth()], year: String(month.value.getFullYear()) }))
const allowed = (d: Date) => (!props.min || iso(d) >= props.min) && (!props.max || iso(d) <= props.max)
const days = computed(() => {
  const first = new Date(month.value.getFullYear(), month.value.getMonth(), 1)
  const start = new Date(first); start.setDate(1 - ((first.getDay() + 6) % 7)) // back to Monday
  return Array.from({ length: 42 }, (_, i) => { const d = new Date(start); d.setDate(start.getDate() + i); return d })
})
const today = iso(new Date())
function pick(d: Date) { if (!allowed(d)) return; model.value = iso(d); open.value = false }

function onDoc(e: MouseEvent) { if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false }
document.addEventListener('mousedown', onDoc)
onBeforeUnmount(() => document.removeEventListener('mousedown', onDoc))
</script>

<template>
  <span ref="root" class="di">
    <input v-model="text" type="text" inputmode="numeric" :placeholder="t('calendar.datePlaceholder')" :aria-label="ariaLabel" :required="required"
           @focus="!open && toggle()" @blur="commit" @keydown.enter.prevent="commit(); open = false" @keydown.esc.stop="open = false" />
    <button type="button" class="di-btn" :aria-label="open ? t('calendar.closeCalendar') : t('calendar.openCalendar')" tabindex="-1" @mousedown.prevent @click="toggle"><AppIcon name="cal" /></button>
    <div v-if="open" class="di-pop" role="dialog" :aria-label="t('calendar.pickDate')" @mousedown.prevent>
      <div class="di-head">
        <button type="button" class="icon-btn" :aria-label="t('calendar.prevMonth')" @click="shift(-1)">‹</button>
        <b>{{ title }}</b>
        <button type="button" class="icon-btn" :aria-label="t('calendar.nextMonth')" @click="shift(1)">›</button>
      </div>
      <div class="di-grid">
        <span v-for="w in weekdayNames()" :key="w" class="di-wd">{{ w }}</span>
        <button v-for="d in days" :key="d.getTime()" type="button" class="di-day"
                :class="{ out: d.getMonth() !== month.getMonth(), on: iso(d) === model, today: iso(d) === today, we: d.getDay() === 0 || d.getDay() === 6 }"
                :disabled="!allowed(d)" @click="pick(d)">{{ d.getDate() }}</button>
      </div>
      <div class="di-foot">
        <button type="button" class="link" :disabled="!allowed(new Date())" @click="pick(new Date())">{{ t('date.today') }}</button>
        <button v-if="!required && model" type="button" class="link" @click="model = ''; open = false">{{ t('calendar.clear') }}</button>
      </div>
    </div>
  </span>
</template>

<style scoped>
.di{position:relative;display:inline-flex;align-items:center;width:100%}
.di input{width:100%;padding-right:34px !important}
.di-btn{position:absolute;right:4px;top:50%;transform:translateY(-50%);border:0;background:none;color:var(--ink-3);cursor:pointer;padding:4px;display:grid;place-items:center;border-radius:4px}
.di-btn:hover{color:var(--accent);background:var(--surface-3)}
.di-btn :deep(svg){width:16px;height:16px}
.di-pop{position:absolute;top:calc(100% + 4px);left:0;z-index:120;width:252px;background:var(--surface);border:1px solid var(--line);border-radius:6px;box-shadow:0 12px 32px rgba(10,20,40,.22);padding:8px}
.di-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}
.di-head b{font-size:13.5px;font-weight:600;text-transform:capitalize}
.di-head .icon-btn{font-size:18px;line-height:1;width:28px;height:28px}
.di-grid{display:grid;grid-template-columns:repeat(7,1fr);gap:2px}
.di-wd{font-size:11px;color:var(--ink-3);text-align:center;padding:2px 0}
.di-day{height:30px;border:0;border-radius:4px;background:none;color:var(--ink);font:inherit;font-size:13px;cursor:pointer}
.di-day:hover:not(:disabled){background:var(--surface-3)}
.di-day.we{color:var(--accent)}
.di-day.out{color:var(--ink-3);opacity:.55}
.di-day.today{box-shadow:inset 0 0 0 1px var(--accent)}
.di-day.on{background:var(--accent);color:#fff;opacity:1}
.di-day:disabled{opacity:.25;cursor:default}
.di-foot{display:flex;justify-content:space-between;margin-top:6px;font-size:12.5px}
</style>
