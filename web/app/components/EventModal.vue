<script setup lang="ts">
// Create or edit a calendar event / task.
import { ref, computed, onMounted, nextTick } from 'vue'
import { cal, saveEvent, deleteEvent, CAL_COLORS, REMINDERS, dayKey, p2 } from '~/composables/calendar'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

const src = cal.editing!
const s0 = new Date(src.start!)
const e0 = src.end ? new Date(src.end) : new Date(s0.getTime() + 3600e3)
const f = ref({
  title: src.title || '', notes: src.notes || '', isTask: !!src.isTask, done: !!src.done, allDay: !!src.allDay, color: src.color || 'blue',
  date: dayKey(s0), from: `${p2(s0.getHours())}:${p2(s0.getMinutes())}`,
  endDate: dayKey(e0), to: `${p2(e0.getHours())}:${p2(e0.getMinutes())}`,
  remind: src.remindMinutes == null ? '' : String(src.remindMinutes),
})
const busy = ref(false)
const err = ref('')
const titleInput = ref<HTMLInputElement | null>(null)
const heading = computed(() => src.id ? (f.value.isTask ? t('calendar.task') : t('calendar.event')) : (f.value.isTask ? t('calendar.newTask') : t('calendar.newEvent')))
onMounted(async () => { await nextTick(); titleInput.value?.focus() })

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

const local = (date: string, time: string) => new Date(`${date}T${time || '00:00'}:00`)
async function save() {
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
      remindMinutes: x.remind === '' ? null : Number(x.remind), mailRef: src.mailRef })
    toast(src.id ? t('common.saved') : (x.isTask ? t('calendar.taskAdded') : t('calendar.eventAdded')))
    cal.editing = null
  } catch (e: any) {
    err.value = e.message
  } finally { busy.value = false }
}
async function remove() {
  if (!src.id || !await confirmDialog(t('calendar.deleteConfirm'), t('calendar.deleteConfirmText', { title: f.value.title }), { ok: t('common.delete'), danger: true })) return
  try { await deleteEvent(src.id); cal.editing = null; toast(t('calendar.deleted')) } catch (e: any) { err.value = e.message }
}
</script>

<template>
  <AppModal :title="heading" width="520px" :busy="busy ? t('common.saving') : ''" @close="cal.editing = null">
    <form class="form ev-form" @submit.prevent="save">
      <input ref="titleInput" v-model="f.title" class="ev-title" type="text" :placeholder="t('calendar.eventTitle')" :aria-label="t('calendar.eventTitle')" />
      <span class="seg" role="group" :aria-label="t('calendar.type')">
        <button type="button" :class="{ on: !f.isTask }" @click="f.isTask = false">{{ t('calendar.event') }}</button>
        <button type="button" :class="{ on: f.isTask }" @click="f.isTask = true">{{ t('calendar.task') }}</button>
      </span>
      <div class="ev-when">
        <label>{{ t('calendar.date') }}<DateInput v-model="f.date" required :aria-label="t('calendar.date')" /></label>
        <template v-if="!f.allDay">
          <label>{{ f.isTask ? t('calendar.due') : t('calendar.from') }}<input v-model="f.from" type="time" /></label>
          <label v-if="!f.isTask">{{ t('calendar.to') }}<input v-model="f.to" type="time" /></label>
        </template>
        <label v-else-if="!f.isTask">{{ t('calendar.until') }}<DateInput v-model="f.endDate" :min="f.date" :aria-label="t('calendar.until')" /></label>
      </div>
      <label class="check"><input v-model="f.allDay" type="checkbox" />{{ t('calendar.allDay') }}</label>
      <label v-if="f.isTask && src.id" class="check"><input v-model="f.done" type="checkbox" />{{ t('calendar.done') }}</label>
      <div class="ev-colors" role="radiogroup" :aria-label="t('calendar.color')">
        <button v-for="(c, k) in CAL_COLORS" :key="k" type="button" class="ev-color" :class="{ on: f.color === k }" :style="{ background: c.bg }"
                :title="colorLabel(k)" :aria-label="colorLabel(k)" role="radio" :aria-checked="f.color === k" @click="f.color = k as string"></button>
      </div>
      <label>{{ t('calendar.reminder') }}<AppSelect v-model="f.remind" :options="reminderOptions" :aria-label="t('calendar.reminder')" /></label>
      <label>{{ t('calendar.notes') }}<textarea v-model="f.notes" rows="4" :placeholder="t('calendar.notesPlaceholder')"></textarea></label>
      <p v-if="err" class="err" role="alert">{{ err }}</p>
    </form>
    <template #foot>
      <button v-if="src.id" class="btn danger-text" type="button" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
      <span class="grow"></span>
      <button class="btn" type="button" @click="cal.editing = null">{{ t('common.cancel') }}</button>
      <button class="btn primary" type="button" :disabled="busy" @click="save">{{ busy ? t('common.saving') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>
