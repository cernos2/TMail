<script setup lang="ts">
// Calendar reminders waiting for confirmation. They stay on screen; without an answer the server
// shows them again every 5 minutes (also as a browser notification).
import { cal, reminderAction, dropReminder, type CalEvent } from '~/composables/calendar'
import { longDate } from '~/composables/format'
import { t, fmtDate } from '~/composables/i18n'

const when = (e: CalEvent) => {
  const start = new Date(e.start)
  const mins = Math.round((start.getTime() - Date.now()) / 60000)
  if (e.allDay) return t('calendar.remTodayAllDay')
  if (mins > 0) return t('calendar.remIn', { n: mins, time: fmtDate(start, { hour: '2-digit', minute: '2-digit' }) })
  if (mins > -1) return t('calendar.remNow')
  return t('calendar.remStarted', { time: fmtDate(start, { hour: '2-digit', minute: '2-digit' }) })
}
function open(e: CalEvent) { cal.editing = { ...e } }
</script>

<template>
  <div v-if="cal.reminders.length" class="rem-stack" role="region" :aria-label="t('calendar.reminders')">
    <article v-for="e in cal.reminders" :key="e.id" class="rem" :class="'c-' + (e.color || 'blue')" role="alertdialog" :aria-label="t('calendar.reminderAria', { title: e.title })">
      <header>
        <AppIcon name="cal" /><span class="rem-kind">{{ e.isTask ? t('calendar.task') : t('calendar.reminder') }}</span>
        <span class="rem-when">{{ when(e) }}</span>
        <button class="icon-btn" :title="t('calendar.remHideTitle')" :aria-label="t('calendar.hide')" @click="dropReminder(e.id)"><AppIcon name="close" /></button>
      </header>
      <b class="rem-title">{{ e.title }}</b>
      <p v-if="e.notes" class="rem-notes">{{ e.notes }}</p>
      <span class="rem-hint">{{ t('calendar.remHint', { date: longDate(e.start) }) }}</span>
      <footer>
        <button class="btn primary sm" @click="reminderAction(e, 'done')"><AppIcon name="check" />{{ e.isTask ? t('calendar.done') : t('calendar.gotIt') }}</button>
        <button class="btn sm" @click="reminderAction(e, 'snooze', 15)">{{ t('calendar.snooze15') }}</button>
        <button class="btn sm" @click="reminderAction(e, 'snooze', 60)">{{ t('calendar.snoozeHour') }}</button>
        <button class="link" @click="open(e)">{{ t('common.open') }}</button>
      </footer>
    </article>
  </div>
</template>

<style scoped>
.rem-stack{position:fixed;right:20px;bottom:20px;z-index:80;display:flex;flex-direction:column;gap:10px;width:min(380px,calc(100vw - 32px));max-height:calc(100vh - 40px);overflow:auto}
.rem{background:var(--surface);border:1px solid var(--line);border-left:4px solid var(--accent);border-radius:6px;box-shadow:0 14px 40px rgba(10,20,40,.22);padding:12px 14px;display:flex;flex-direction:column;gap:6px;animation:rem-in .25s ease-out}
.rem.c-red{border-left-color:#D64545}.rem.c-green{border-left-color:#2E9E5B}.rem.c-yellow{border-left-color:#E0A526}.rem.c-purple{border-left-color:#8A4B9C}.rem.c-orange{border-left-color:#E07A26}.rem.c-teal{border-left-color:#1E8C8C}.rem.c-gray{border-left-color:#6B7480}
.rem header{display:flex;align-items:center;gap:6px;font-size:12px;color:var(--ink-3)}
.rem header svg{width:14px;height:14px}
.rem-kind{font-weight:600;text-transform:uppercase;letter-spacing:.04em}
.rem-when{margin-left:auto;color:var(--accent);font-weight:600}
.rem-title{font-size:15px}
.rem-notes{margin:0;font-size:13px;color:var(--ink-2);white-space:pre-wrap;max-height:80px;overflow:auto}
.rem-hint{font-size:11.5px;color:var(--ink-3)}
.rem footer{display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin-top:4px}
@keyframes rem-in{from{transform:translateY(12px);opacity:0}to{transform:none;opacity:1}}
</style>
