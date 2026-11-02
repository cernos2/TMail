// Personal calendar and monthly planner. Shown in the reading pane whenever no letter is open.
import { reactive, computed } from 'vue'
import { api } from './api'
import { toast } from './toast'
import { notifyState } from './notify'
import { t, fmtDate, monthNames, weekdayNames, i18n } from './i18n'

export type CalEvent = {
  id: number, title: string, notes: string, start: string, end?: string | null, allDay: boolean,
  color: string, isTask: boolean, done: boolean, remindMinutes?: number | null, mailRef?: string,
  calendarId?: number, rrule?: string, occurrence?: string | null, canEdit?: boolean,
  seriesStart?: string | null, seriesEnd?: string | null,
}
// A calendar the user sees: own ones and colleagues' shared with them.
export type Calendar = {
  id: number, name: string, color: string, mine: boolean, canWrite: boolean, hidden: boolean, owner: string,
  icsUrl?: string, shares?: { userId: number, login: string, name: string, canWrite: boolean }[], davPath: string,
}
// Repeat choices of the event form (iCalendar rules); a phone may set any other rule, shown as such.
export const REPEATS = ['', 'FREQ=DAILY', 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR', 'FREQ=WEEKLY', 'FREQ=MONTHLY', 'FREQ=YEARLY']
export const evKey = (e: CalEvent) => `${e.id}|${e.occurrence || ''}`
export type PlanItem = { text: string, done: boolean }
export type Plan = { goals: PlanItem[], tasks: PlanItem[], notes: string }

// Labels are getters: read at render time, so they follow a language switch.
const color = (key: string, bg: string, fg: string) => ({ get label() { return t(`calendar.color.${key}`) }, bg, fg })
export const CAL_COLORS: Record<string, { label: string, bg: string, fg: string }> = {
  blue: color('blue', '#A9D1F0', '#0E2F4A'), green: color('green', '#9BDDBB', '#0D3B26'),
  yellow: color('yellow', '#F6D88A', '#4A3500'), red: color('red', '#F4A9A3', '#4D0F0A'),
  purple: color('purple', '#C9B8EE', '#2A1857'), teal: color('teal', '#93D8D5', '#08393A'),
  orange: color('orange', '#F8C29A', '#4D2300'), gray: color('gray', '#C7CDD6', '#1E2530'),
}
const remind = (value: string, label: () => string) => ({ value, get label() { return label() } })
export const REMINDERS: { value: string, label: string }[] = [
  remind('', () => t('calendar.remind.none')), remind('0', () => t('calendar.remind.atStart')),
  remind('5', () => t('calendar.remind.minutes', { n: 5 })), remind('15', () => t('calendar.remind.minutes', { n: 15 })),
  remind('30', () => t('calendar.remind.minutes', { n: 30 })), remind('60', () => t('calendar.remind.hours', { n: 1 })),
  remind('180', () => t('calendar.remind.hours', { n: 3 })), remind('1440', () => t('calendar.remind.days', { n: 1 })),
  remind('10080', () => t('calendar.remind.week')),
]
// Month names (January first) and short weekday names (Monday first) in the user's language.
// Kept as arrays for older callers; new code should call monthNames() / weekdayNames() directly.
const live = (names: () => string[]) => new Proxy([] as string[], { get: (_, k) => Reflect.get(names(), k), has: (_, k) => Reflect.has(names(), k) })
export const MONTHS: string[] = live(() => monthNames('long'))
export const WEEKDAYS: string[] = live(() => weekdayNames('short'))

const today0 = () => { const d = new Date(); d.setHours(0, 0, 0, 0); return d }

export const cal = reactive({
  month: (() => { const d = today0(); d.setDate(1); return d })(),
  events: [] as CalEvent[],
  plan: { goals: [], tasks: [], notes: '' } as Plan,
  loading: false,
  editing: null as null | Partial<CalEvent>,
  planOpen: true,
  reminders: [] as CalEvent[], // shown until confirmed; the server repeats them every 5 minutes
  calendars: [] as Calendar[],
  calEditing: null as null | Partial<Calendar>,
})

export const calName = (c?: Calendar) => !c ? '' : (c.name || t('calendar.personal')) + (c.mine ? '' : ` · ${c.owner}`)
export const writableCalendars = () => cal.calendars.filter(c => c.canWrite)

export async function loadCalendars() {
  try { cal.calendars = (await api<{ calendars: Calendar[] }>('/api/calendars')).calendars } catch {}
}

export async function saveCalendar(c: Partial<Calendar> & { shares?: Calendar['shares'], ics?: boolean, rotate?: boolean }) {
  const body: Record<string, any> = { name: c.name, color: c.color }
  if (c.shares) body.shares = c.shares
  if (c.ics !== undefined) body.ics = c.ics
  if (c.rotate) body.rotate = true
  const r = c.id ? await api<{ icsUrl?: string }>(`/api/calendars/${c.id}`, { method: 'PUT', body })
    : await api<{ id: number }>('/api/calendars', { method: 'POST', body })
  await loadCalendars()
  return r
}

export async function setCalendarHidden(c: Calendar, hidden: boolean) {
  c.hidden = hidden
  try { await api(`/api/calendars/${c.id}`, { method: 'PUT', body: { hidden } }); loadCalendar() }
  catch (e: any) { c.hidden = !hidden; toast(e.message, { kind: 'err' }) }
}

export async function deleteCalendar(c: Calendar) {
  await api(`/api/calendars/${c.id}`, { method: 'DELETE' })
  await loadCalendars()
  loadCalendar()
}
try { cal.planOpen = localStorage.getItem('tmail.planOpen') !== '0' } catch {}

export const p2 = (n: number) => String(n).padStart(2, '0')
export const dayKey = (d: Date) => `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())}`
export const monthKey = (d: Date) => `${d.getFullYear()}-${p2(d.getMonth() + 1)}`
export const timeOf = (iso: string) => fmtDate(iso, { hour: '2-digit', minute: '2-digit' })

// Six weeks starting on the Monday on or before the 1st.
export const gridDays = computed(() => {
  const first = new Date(cal.month)
  const start = new Date(first)
  start.setDate(1 - ((first.getDay() + 6) % 7))
  return Array.from({ length: 42 }, (_, i) => { const d = new Date(start); d.setDate(start.getDate() + i); return d })
})

export const eventsByDay = computed(() => {
  const out: Record<string, CalEvent[]> = {}
  for (const e of cal.events) {
    const s = new Date(e.start); s.setHours(0, 0, 0, 0)
    const end = e.end ? new Date(e.end) : new Date(e.start)
    // Multi-day events appear on every day they cover (an all-day end is the last day itself).
    for (let d = new Date(s), n = 0; d <= end && n < 62; d.setDate(d.getDate() + 1), n++) {
      (out[dayKey(d)] ||= []).push(e)
    }
  }
  for (const k in out) out[k].sort((a, b) => Number(b.allDay) - Number(a.allDay) || a.start.localeCompare(b.start))
  return out
})

let loadSeq = 0
export async function loadCalendar() {
  if (!cal.calendars.length) loadCalendars()
  const seq = ++loadSeq
  const days = gridDays.value
  const from = days[0]
  const to = new Date(days[days.length - 1]); to.setDate(to.getDate() + 1)
  cal.loading = true
  try {
    const [events, plan] = await Promise.all([
      api<CalEvent[]>('/api/calendar/events', { query: { from: from.toISOString(), to: to.toISOString() } }),
      api<Plan>('/api/calendar/plan', { query: { m: monthKey(cal.month) } }),
    ])
    if (seq !== loadSeq) return
    cal.events = events
    cal.plan = plan
  } catch (e: any) {
    if (seq === loadSeq) toast(e.message, { kind: 'err' })
  } finally {
    if (seq === loadSeq) cal.loading = false
  }
}

export function shiftMonth(n: number) {
  flushPlan()
  const d = new Date(cal.month); d.setMonth(d.getMonth() + n); cal.month = d
  loadCalendar()
}
export function goToday() {
  flushPlan()
  const d = today0(); d.setDate(1); cal.month = d
  loadCalendar()
}

export function newEvent(day?: Date, extra: Partial<CalEvent> = {}) {
  const d = day ? new Date(day) : new Date()
  if (day) d.setHours(9, 0, 0, 0)
  else d.setMinutes(0, 0, 0), d.setHours(d.getHours() + 1)
  const end = new Date(d); end.setHours(d.getHours() + 1)
  cal.editing = { title: '', notes: '', start: d.toISOString(), end: end.toISOString(), allDay: false, color: 'blue', isTask: false, done: false, remindMinutes: 15, ...extra }
}

export async function saveEvent(e: Partial<CalEvent>) {
  const { occurrence: _o, canEdit: _c, ...rest } = e
  const body = { ...rest, remindMinutes: e.remindMinutes ?? null, end: e.end || null, rrule: e.rrule || '' }
  const saved = e.id
    ? await api<CalEvent>(`/api/calendar/events/${e.id}`, { method: 'PUT', body })
    : await api<CalEvent>('/api/calendar/events', { method: 'POST', body })
  // A repeating event (or one that stopped repeating) changes many days: read the month again.
  if (saved.rrule || e.rrule || cal.events.some(x => x.id === saved.id && x.occurrence)) { loadCalendar(); return saved }
  const i = cal.events.findIndex(x => x.id === saved.id)
  if (i >= 0) cal.events[i] = saved
  else cal.events.push(saved)
  return saved
}

// occurrence: delete only this repetition of a repeating event.
export async function deleteEvent(id: number, occurrence?: string | null) {
  await api(`/api/calendar/events/${id}`, { method: 'DELETE', query: occurrence ? { occurrence } : undefined })
  cal.events = cal.events.filter(e => e.id !== id || (occurrence && e.occurrence !== occurrence))
}

export async function toggleDone(e: CalEvent) {
  if (e.canEdit === false) return
  e.done = !e.done
  // A repetition carries its own date: the series keeps the start it has.
  const series = e.occurrence ? cal.events.find(x => x.id === e.id && !x.occurrence) : null
  const body = e.occurrence ? { ...e, start: series?.start || e.start, end: series ? series.end : e.end } : e
  try { await saveEvent(body) } catch (err: any) { e.done = !e.done; toast(err.message, { kind: 'err' }) }
}

// Planner page autosaves a moment after the last change.
let planTimer: ReturnType<typeof setTimeout> | null = null
let planMonth = ''
export function planChanged() {
  planMonth = monthKey(cal.month)
  if (planTimer) clearTimeout(planTimer)
  planTimer = setTimeout(flushPlan, 700)
}
export function flushPlan() {
  if (!planTimer) return
  clearTimeout(planTimer)
  planTimer = null
  const body = { goals: cal.plan.goals.filter(x => x.text.trim()), tasks: cal.plan.tasks.filter(x => x.text.trim()), notes: cal.plan.notes }
  api('/api/calendar/plan', { method: 'PUT', query: { m: planMonth }, body }).catch((e: any) => toast(e.message, { kind: 'err' }))
}

// Reminder pushed by the server: browser notification (same look as new mail) and an in-app toast.
export function showReminder(e: CalEvent) {
  const when = e.allDay ? t('calendar.today') : timeOf(e.start)
  const text = t('calendar.reminderText', { title: e.title, when })
  const i = cal.reminders.findIndex(x => x.id === e.id)
  if (i >= 0) cal.reminders.splice(i, 1)
  cal.reminders.unshift(e) // a repeated reminder comes to the top again
  if (notifyState.calendar && notifyState.supported && notifyState.permission === 'granted') {
    try {
      const n = new Notification(i >= 0 ? t('calendar.reminderAgain') : t('calendar.reminder'), {
        body: `${text}${e.notes ? '\n' + e.notes.slice(0, 120) : ''}\n${t('calendar.reminderConfirm')}`,
        icon: '/icon-192.png', tag: `cal-${e.id}`, renotify: true, requireInteraction: true, lang: i18n.lang,
      } as NotificationOptions)
      n.onclick = () => { window.focus(); n.close() }
    } catch {}
  }
}

export function dropReminder(id: number) {
  const i = cal.reminders.findIndex(x => x.id === id)
  if (i >= 0) cal.reminders.splice(i, 1)
}

// done — task done / event handled; snooze — remind again in N minutes.
export async function reminderAction(e: CalEvent, action: 'done' | 'snooze', minutes = 15) {
  try {
    await api(`/api/calendar/events/${e.id}/reminder`, { method: 'POST', body: { action, minutes } })
    dropReminder(e.id)
    if (action === 'done') { const ev = cal.events.find(x => x.id === e.id); if (ev && ev.isTask) ev.done = true }
    toast(action === 'done' ? (e.isTask ? t('calendar.taskDone') : t('calendar.reminderClosed')) : minutes >= 60 ? t('calendar.snoozedHours', { n: minutes / 60 }) : t('calendar.snoozedMinutes', { n: minutes }))
  } catch (err: any) { toast(err.message, { kind: 'err' }) }
}
