<script setup lang="ts">
// Month calendar with a planner page beside it (goals, important tasks, upcoming dates, notes).
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { mail } from '~/composables/mail'
import { cal, gridDays, eventsByDay, loadCalendar, shiftMonth, goToday, newEvent, toggleDone, planChanged, flushPlan,
  dayKey, CAL_COLORS, type CalEvent } from '~/composables/calendar'
import { t, fmtDate, monthNames, weekdayNames } from '~/composables/i18n'

const todayKey = dayKey(new Date())
const title = computed(() => t('calendar.monthYear', { month: monthNames()[cal.month.getMonth()], year: String(cal.month.getFullYear()) }))
const isThisMonth = computed(() => { const n = new Date(); return n.getMonth() === cal.month.getMonth() && n.getFullYear() === cal.month.getFullYear() })
const MAX = 3
const dayOpen = ref<Date | null>(null)

const upcoming = computed(() => {
  const now = new Date().toISOString()
  return cal.events.filter(e => (e.end || e.start) >= now && !e.done).sort((a, b) => a.start.localeCompare(b.start)).slice(0, 6)
})
const chipStyle = (e: CalEvent) => ({ background: CAL_COLORS[e.color]?.bg, color: CAL_COLORS[e.color]?.fg })
const edit = (e: CalEvent) => { cal.editing = { ...e } }
const shortDate = (iso: string) => fmtDate(iso, { day: 'numeric', month: 'short' })
const timeOf = (iso: string) => fmtDate(iso, { hour: '2-digit', minute: '2-digit' })
// Grid days are local midnights; noon keeps the shown date right in a nearby time zone.
const noon = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), 12)

function addItem(list: 'goals' | 'tasks') { cal.plan[list].push({ text: '', done: false }); planChanged() }
function removeItem(list: 'goals' | 'tasks', i: number) { cal.plan[list].splice(i, 1); planChanged() }
function togglePlan() { cal.planOpen = !cal.planOpen; try { localStorage.setItem('tmail.planOpen', cal.planOpen ? '1' : '0') } catch {} }

onMounted(loadCalendar)
onBeforeUnmount(flushPlan)
</script>

<template>
  <div class="calv">
    <header class="cal-head">
      <button class="icon-btn back-btn" :aria-label="t('calendar.backToMail')" @click="mail.reading = false"><AppIcon name="back" /></button>
      <span class="cal-nav">
        <button class="icon-btn" :title="t('calendar.prevMonth')" :aria-label="t('calendar.prevMonth')" @click="shiftMonth(-1)"><AppIcon name="back" /></button>
        <h2 class="cal-month-title" aria-live="polite">{{ title }}</h2>
        <button class="icon-btn flip" :title="t('calendar.nextMonth')" :aria-label="t('calendar.nextMonth')" @click="shiftMonth(1)"><AppIcon name="back" /></button>
      </span>
      <button v-if="!isThisMonth" class="btn sm" :title="t('calendar.backToThisMonth')" @click="goToday">{{ t('date.today') }}</button>
      <span class="grow"></span>
      <button class="btn sm primary" @click="newEvent()"><AppIcon name="plus" />{{ t('calendar.event') }}</button>
      <button class="icon-btn" :class="{ on: cal.planOpen }" :title="cal.planOpen ? t('calendar.hidePlanner') : t('calendar.showPlanner')" :aria-label="t('calendar.planner')" @click="togglePlan"><AppIcon name="list" /></button>
    </header>

    <div class="cal-main veil-host" :class="{ 'with-plan': cal.planOpen }">
      <LoadingVeil :show="cal.loading" :text="t('calendar.loading')" />
      <div class="cal-grid" role="grid" :aria-label="title">
        <div v-for="(w, i) in weekdayNames()" :key="w" class="cal-wd" :class="{ we: i > 4 }">{{ w }}</div>
        <div v-for="(d, i) in gridDays" :key="i" class="cal-cell" role="gridcell"
             :class="{ out: d.getMonth() !== cal.month.getMonth(), we: i % 7 > 4, today: dayKey(d) === todayKey }"
             @click.self="newEvent(d)">
          <button type="button" class="cal-num" :title="t('calendar.newEventOn', { date: fmtDate(noon(d), { day: 'numeric', month: 'numeric', year: 'numeric' }) })" @click="newEvent(d)">{{ d.getDate() }}</button>
          <button v-for="e in (eventsByDay[dayKey(d)] || []).slice(0, MAX)" :key="e.id" type="button" class="cal-chip" :class="{ done: e.done }"
                  :style="chipStyle(e)" :title="e.title" @click="edit(e)">
            <input v-if="e.isTask" type="checkbox" :checked="e.done" :aria-label="t('calendar.doneAria', { title: e.title })" @click.stop="toggleDone(e)" />
            <span v-if="!e.allDay" class="cal-chip-time">{{ timeOf(e.start) }}</span>
            <span class="cal-chip-t">{{ e.title }}</span>
          </button>
          <button v-if="(eventsByDay[dayKey(d)] || []).length > MAX" type="button" class="cal-more" @click="dayOpen = d">
            {{ t('calendar.moreN', { n: (eventsByDay[dayKey(d)] || []).length - MAX }) }}</button>
        </div>
      </div>

      <aside v-if="cal.planOpen" class="plan" :aria-label="t('calendar.monthPlanner')">
        <section class="plan-box goals">
          <h3><AppIcon name="star" />{{ t('calendar.goals') }}</h3>
          <ol>
            <li v-for="(g, i) in cal.plan.goals" :key="'g' + i">
              <input v-model="g.text" type="text" :placeholder="t('calendar.goalPlaceholder')" @input="planChanged" />
              <button class="x" type="button" :aria-label="t('calendar.remove')" @click="removeItem('goals', i)"><AppIcon name="close" /></button>
            </li>
          </ol>
          <button class="link" type="button" @click="addItem('goals')">{{ t('calendar.addGoal') }}</button>
        </section>
        <section class="plan-box tasks">
          <h3><AppIcon name="check" />{{ t('calendar.tasks') }}</h3>
          <ul>
            <li v-for="(task, i) in cal.plan.tasks" :key="'t' + i" :class="{ done: task.done }">
              <input v-model="task.done" type="checkbox" :aria-label="t('calendar.doneAria', { title: task.text })" @change="planChanged" />
              <input v-model="task.text" type="text" :placeholder="t('calendar.taskPlaceholder')" @input="planChanged" />
              <button class="x" type="button" :aria-label="t('calendar.remove')" @click="removeItem('tasks', i)"><AppIcon name="close" /></button>
            </li>
          </ul>
          <button class="link" type="button" @click="addItem('tasks')">{{ t('calendar.addTask') }}</button>
        </section>
        <section class="plan-box dates">
          <h3><AppIcon name="cal" />{{ t('calendar.upcoming') }}</h3>
          <p v-if="!upcoming.length" class="hint">{{ t('calendar.nothingPlanned') }}</p>
          <button v-for="e in upcoming" :key="e.id" type="button" class="plan-ev" @click="edit(e)">
            <i :style="{ background: CAL_COLORS[e.color]?.bg }"></i><b>{{ shortDate(e.start) }}{{ e.allDay ? '' : ' ' + timeOf(e.start) }}</b><span>{{ e.title }}</span>
          </button>
        </section>
        <section class="plan-box notes">
          <h3><AppIcon name="edit" />{{ t('calendar.notes') }}</h3>
          <textarea v-model="cal.plan.notes" :placeholder="t('calendar.monthNotesPlaceholder')" @input="planChanged"></textarea>
        </section>
      </aside>
    </div>

    <AppModal v-if="dayOpen" :title="fmtDate(noon(dayOpen), { day: 'numeric', month: 'long', weekday: 'long' })" width="420px" @close="dayOpen = null">
      <div class="day-list">
        <button v-for="e in eventsByDay[dayKey(dayOpen)] || []" :key="e.id" type="button" class="cal-chip big" :class="{ done: e.done }" :style="chipStyle(e)" @click="edit(e); dayOpen = null">
          <span class="cal-chip-t">{{ e.title }}</span><span v-if="!e.allDay" class="cal-chip-time">{{ timeOf(e.start) }}</span>
        </button>
        <button class="btn sm" @click="newEvent(dayOpen!); dayOpen = null"><AppIcon name="plus" />{{ t('calendar.newEvent') }}</button>
      </div>
    </AppModal>
  </div>
</template>
