<script setup lang="ts">
// Create or edit a calendar event / task: its calendar, time, repetition, colour and reminder.
// A repeating event is edited as a whole series; one repetition can be deleted on its own.
import { ref, computed, onMounted, nextTick } from 'vue'
import { cal, saveEvent, deleteEvent, CAL_COLORS, REMINDERS, REPEATS, dayKey, p2, calName, writableCalendars, loadCalendars } from '~/composables/calendar'
import { confirmDialog, choiceDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t, fmtDate, weekdayNames } from '~/composables/i18n'

const src = cal.editing!
const readOnly = src.id ? src.canEdit === false : false
// A repetition opens the series: its first date and time are edited.
const s0 = new Date(src.occurrence && src.seriesStart ? src.seriesStart : src.start!)
const e0 = (src.occurrence && src.seriesStart ? (src.seriesEnd ? new Date(src.seriesEnd) : null) : (src.end ? new Date(src.end) : null)) || new Date(s0.getTime() + 3600e3)
const f = ref({
  title: src.title || '', notes: src.notes || '', isTask: !!src.isTask, done: !!src.done, allDay: !!src.allDay, color: src.color || 'blue',
  date: dayKey(s0), from: `${p2(s0.getHours())}:${p2(s0.getMinutes())}`,
  endDate: dayKey(e0), to: `${p2(e0.getHours())}:${p2(e0.getMinutes())}`,
  remind: src.remindMinutes == null ? '' : String(src.remindMinutes),
  calendarId: src.calendarId || writableCalendars()[0]?.id || 0,
  rrule: src.rrule || '',
})
const busy = ref(false)
const err = ref('')
const titleInput = ref<HTMLInputElement | null>(null)
const heading = computed(() => readOnly ? (f.value.isTask ? t('calendar.task') : t('calendar.event'))
  : src.id ? (f.value.isTask ? t('calendar.task') : t('calendar.event')) : (f.value.isTask ? t('calendar.newTask') : t('calendar.newEvent')))
onMounted(async () => { if (!cal.calendars.length) await loadCalendars(); if (!f.value.calendarId) f.value.calendarId = writableCalendars()[0]?.id || 0; await nextTick(); if (!readOnly) titleInput.value?.focus() })

// Color names and reminder choices in the interface language (the values come from composables/calendar).
const colorLabel = (k: string | number) => t('calendar.color' + String(k).charAt(0).toUpperCase() + String(k).slice(1))
function remindLabel(v: string) {
  if (v === '') return t('calendar.remindNone')
  const m = Number(v)
  if (m === 0) return t('calendar.remindAtStart')
  if (m === 10080) return t('calendar.remindWeek')
  if (m % 1440 === 0) return t('calendar.remindDays', { n: m / 1440 })
  if (m % 60 === 0) return t('calendar.remindHours', { n: m / 60 })
  return t('calendar.remindMinutes', { n: m })
}
const reminderOptions = computed(() => REMINDERS.map(r => ({ value: r.value, label: remindLabel(r.value) })))
const calendarOptions = computed(() => (readOnly ? cal.calendars : writableCalendars()).map(c => ({ value: c.id, label: calName(c) })))
// "Every week on Monday", "Every month on the 2nd" — read from the chosen date.
const repeatOptions = computed(() => {
  const d = new Date(`${f.value.date}T12:00:00`)
  const wd = weekdayNames('long')[(d.getDay() + 6) % 7]
  const labels: Record<string, string> = {
    '': t('calendar.repeatNone'), 'FREQ=DAILY': t('calendar.repeatDaily'), 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR': t('calendar.repeatWeekdays'),
    'FREQ=WEEKLY': t('calendar.repeatWeekly', { day: wd }), 'FREQ=MONTHLY': t('calendar.repeatMonthly', { n: d.getDate() }),
    'FREQ=YEARLY': t('calendar.repeatYearly', { date: fmtDate(d, { day: 'numeric', month: 'long' }) }),
  }
  const out = REPEATS.map(v => ({ value: v, label: labels[v] }))
  if (f.value.rrule && !REPEATS.includes(f.value.rrule)) out.push({ value: f.value.rrule, label: t('calendar.repeatCustom') })
  return out
})

const local = (date: string, time: string) => new Date(`${date}T${time || '00:00'}:00`)
async function save() {
  if (readOnly) return
  const x = f.value
  if (!x.title.trim()) { err.value = t('calendar.titleRequired'); return }
  const start = x.allDay ? local(x.date, '00:00') : local(x.date, x.from)
  let end: Date | null = x.allDay ? local(x.endDate < x.date ? x.date : x.endDate, '00:00') : local(x.date, x.to)
  if (!x.allDay && end <= start) end = null
  if (x.isTask && !x.allDay) end = null
  busy.value = true
  err.value = ''
  try {
    await saveEvent({ id: src.id, title: x.title.trim(), notes: x.notes, isTask: x.isTask, done: x.done, allDay: x.allDay, color: x.color,
      start: start.toISOString(), end: end && (x.allDay ? end.getTime() !== start.getTime() : true) ? end.toISOString() : null,
      remindMinutes: x.remind === '' ? null : Number(x.remind), mailRef: src.mailRef, calendarId: x.calendarId || undefined, rrule: x.rrule })
    toast(src.id ? t('common.saved') : (x.isTask ? t('calendar.taskAdded') : t('calendar.eventAdded')))
    cal.editing = null
  } catch (e: any) {
    err.value = e.message
  } finally { busy.value = false }
}
async function remove() {
  if (!src.id) return
  let only = false
  if (src.occurrence) {
    const pick = await choiceDialog(t('calendar.deleteRepeating'), t('calendar.deleteRepeatingText', { title: f.value.title }),
      [{ id: 'one', label: t('calendar.deleteThisOne') }, { id: 'all', label: t('calendar.deleteAll'), danger: true }])
    if (!pick) return
    only = pick === 'one'
  } else if (!await confirmDialog(t('calendar.deleteConfirm'), t('calendar.deleteConfirmText', { title: f.value.title }), { ok: t('common.delete'), danger: true })) return
  try { await deleteEvent(src.id, only ? src.occurrence : null); cal.editing = null; toast(t('calendar.deleted')) } catch (e: any) { err.value = e.message }
}
</script>

<template>
  <AppModal :title="heading" width="540px" :busy="busy ? t('common.saving') : ''" @close="cal.editing = null">
    <form class="form ev-form" @submit.prevent="save">
      <p v-if="readOnly" class="notice"><AppIcon name="lock" />{{ t('calendar.readOnlyEvent') }}</p>
      <input ref="titleInput" v-model="f.title" class="ev-title" type="text" :readonly="readOnly" :placeholder="t('calendar.eventTitle')" :aria-label="t('calendar.eventTitle')" />
      <span class="seg" role="group" :aria-label="t('calendar.type')">
        <button type="button" :class="{ on: !f.isTask }" :disabled="readOnly" @click="f.isTask = false">{{ t('calendar.event') }}</button>
        <button type="button" :class="{ on: f.isTask }" :disabled="readOnly" @click="f.isTask = true">{{ t('calendar.task') }}</button>
      </span>
      <div class="ev-when">
        <label>{{ t('calendar.date') }}<DateInput v-model="f.date" required :aria-label="t('calendar.date')" /></label>
        <template v-if="!f.allDay">
          <label>{{ f.isTask ? t('calendar.due') : t('calendar.from') }}<input v-model="f.from" type="time" :readonly="readOnly" /></label>
          <label v-if="!f.isTask">{{ t('calendar.to') }}<input v-model="f.to" type="time" :readonly="readOnly" /></label>
        </template>
        <label v-else-if="!f.isTask">{{ t('calendar.until') }}<DateInput v-model="f.endDate" :min="f.date" :aria-label="t('calendar.until')" /></label>
      </div>
      <label class="check"><input v-model="f.allDay" type="checkbox" :disabled="readOnly" />{{ t('calendar.allDay') }}</label>
      <label v-if="f.isTask && src.id" class="check"><input v-model="f.done" type="checkbox" :disabled="readOnly" />{{ t('calendar.done') }}</label>
      <div class="ev-grid">
        <label><span><AppIcon name="repeat" />{{ t('calendar.repeat') }}</span><AppSelect v-model="f.rrule" :options="repeatOptions" :aria-label="t('calendar.repeat')" :disabled="readOnly" /></label>
        <label v-if="calendarOptions.length > 1 || readOnly"><span><AppIcon name="cal" />{{ t('calendar.calendar') }}</span><AppSelect v-model="f.calendarId" :options="calendarOptions" :aria-label="t('calendar.calendar')" :disabled="readOnly" /></label>
      </div>
      <p v-if="src.occurrence" class="hint ev-series">{{ t('calendar.seriesNote') }}</p>
      <div class="ev-colors" role="radiogroup" :aria-label="t('calendar.color')">
        <button v-for="(c, k) in CAL_COLORS" :key="k" type="button" class="ev-color" :class="{ on: f.color === k }" :style="{ background: c.bg }" :disabled="readOnly"
                :title="colorLabel(k)" :aria-label="colorLabel(k)" role="radio" :aria-checked="f.color === k" @click="f.color = k as string"></button>
      </div>
      <label>{{ t('calendar.reminder') }}<AppSelect v-model="f.remind" :options="reminderOptions" :aria-label="t('calendar.reminder')" :disabled="readOnly" /></label>
      <label>{{ t('calendar.notes') }}<textarea v-model="f.notes" rows="4" :readonly="readOnly" :placeholder="t('calendar.notesPlaceholder')"></textarea></label>
      <p v-if="err" class="err" role="alert">{{ err }}</p>
    </form>
    <template #foot>
      <button v-if="src.id && !readOnly" class="btn danger-text" type="button" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
      <span class="grow"></span>
      <button class="btn" type="button" @click="cal.editing = null">{{ readOnly ? t('common.close') : t('common.cancel') }}</button>
      <button v-if="!readOnly" class="btn primary" type="button" :disabled="busy" @click="save">{{ busy ? t('common.saving') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>

<style scoped>
.ev-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.ev-grid label > span{display:inline-flex;align-items:center;gap:6px}
.ev-grid label > span svg{width:14px;height:14px;color:var(--ink-3)}
.ev-series{margin:-4px 0 0;font-size:12.5px}
.notice svg{width:16px;height:16px}
@media (max-width:560px){.ev-grid{grid-template-columns:1fr}}
</style>
