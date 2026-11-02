// "Send later" and "Snooze": what is waiting on the server and the times offered in the pickers.
import { reactive } from 'vue'
import { api } from './api'
import { toast } from './toast'
import { t, fmtDate } from './i18n'
import { mail, refreshFoldersSoon, loadMessages, setView, openMessage, keyOf, type Summary } from './mail'

export type Scheduled = { id: number, account: string, messageId: string, subject: string, sendAt: string, status: 'pending' | 'sending' | 'failed', error?: string }
export type Snoozed = { account: string, messageId: string, until: string, folder: string }

export const later = reactive({
  scheduled: [] as Scheduled[],
  snoozed: [] as Snoozed[],
})

const norm = (id: string) => id.trim().replace(/^<|>$/g, '').toLowerCase()

let loadSeq = 0
export async function loadLater() {
  const seq = ++loadSeq
  try {
    const r = await api<{ scheduled: Scheduled[], snoozed: Snoozed[] }>('/api/scheduled')
    if (seq !== loadSeq) return
    later.scheduled = r.scheduled
    later.snoozed = r.snoozed
  } catch {}
}

export const scheduledOf = (m: { account: string, messageId?: string }) =>
  m.messageId ? later.scheduled.find(x => x.account === m.account && norm(x.messageId) === norm(m.messageId!)) : undefined
export const snoozedOf = (m: { account: string, messageId?: string }) =>
  m.messageId ? later.snoozed.find(x => x.account === m.account && norm(x.messageId) === norm(m.messageId!)) : undefined

// "Tomorrow, 09:00" / "Mon, 3 Nov, 08:00" in the user's language and zone; mid: inside a sentence.
export function whenLabel(d: Date | string, mid = false) {
  const x = typeof d === 'string' ? new Date(d) : d
  const now = new Date()
  const day = (a: Date) => `${a.getFullYear()}-${a.getMonth()}-${a.getDate()}`
  const tomorrow = new Date(now); tomorrow.setDate(now.getDate() + 1)
  const time = fmtDate(x, { hour: '2-digit', minute: '2-digit' })
  if (day(x) === day(now)) return t(mid ? 'later.todayAtMid' : 'later.todayAt', { time })
  if (day(x) === day(tomorrow)) return t(mid ? 'later.tomorrowAtMid' : 'later.tomorrowAt', { time })
  return fmtDate(x, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
    ...(x.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) })
}

export type WhenPreset = { key: string, label: string, at: Date }

const at = (base: Date, days: number, h: number, m = 0) => { const d = new Date(base); d.setDate(d.getDate() + days); d.setHours(h, m, 0, 0); return d }

// Snooze choices, like Gmail: later today, tomorrow, the weekend, next week.
export function snoozePresets(): WhenPreset[] {
  const now = new Date()
  const out: WhenPreset[] = []
  const later3 = new Date(now.getTime() + 3 * 3600e3); later3.setMinutes(0, 0, 0)
  if (later3.getHours() >= 8 && later3.getDate() === now.getDate()) out.push({ key: 'later', label: t('later.laterToday'), at: later3 })
  out.push({ key: 'tomorrow', label: t('later.tomorrow'), at: at(now, 1, 9) })
  const dow = now.getDay() // 0 Sunday
  if (dow >= 1 && dow <= 4) out.push({ key: 'weekend', label: t('later.weekend'), at: at(now, 6 - dow, 9) })
  out.push({ key: 'week', label: t('later.nextWeek'), at: at(now, ((8 - dow) % 7) || 7, 9) })
  return out
}

// Send-later choices: tomorrow morning and afternoon, Monday morning.
export function sendPresets(): WhenPreset[] {
  const now = new Date()
  const dow = now.getDay()
  const out: WhenPreset[] = [
    { key: 'morning', label: t('later.tomorrowMorning'), at: at(now, 1, 8) },
    { key: 'afternoon', label: t('later.tomorrowAfternoon'), at: at(now, 1, 13) },
  ]
  if (dow !== 0 && dow !== 6) out.push({ key: 'monday', label: t('later.mondayMorning'), at: at(now, ((8 - dow) % 7) || 7, 8) })
  return out
}

export async function snooze(items: Summary[], until: Date) {
  const byFolder = new Map<string, Summary[]>()
  for (const m of items) {
    const k = `${m.account}\n${m.folder}`
    byFolder.set(k, [...(byFolder.get(k) || []), m])
  }
  try {
    for (const list of byFolder.values()) {
      await api('/api/messages/snooze', { method: 'POST', body: { account: list[0].account, folder: list[0].folder, uids: list.map(m => m.uid), until: until.toISOString() } })
    }
    const gone = new Set(items.map(keyOf))
    mail.messages = mail.messages.filter(m => !gone.has(keyOf(m)))
    if (mail.current && gone.has(keyOf(mail.current))) { mail.current = null; mail.selected = null }
    toast(t('later.snoozedUntil', { when: whenLabel(until, true) }), { action: { label: t('later.openSnoozed'), run: () => setView({ role: 'snoozed' }) } })
    refreshFoldersSoon(300)
    loadLater()
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}

export async function unsnooze(m: Summary) {
  try {
    await api('/api/messages/unsnooze', { method: 'POST', body: { account: m.account, messageId: m.messageId } })
    toast(t('later.backInInbox'))
    if (mail.current && keyOf(mail.current) === keyOf(m)) { mail.current = null; mail.selected = null }
    loadMessages(false, true)
    refreshFoldersSoon(300)
    loadLater()
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}

export async function scheduledAction(s: Scheduled, action: 'cancel' | 'now' | 'reschedule', sendAt?: Date) {
  try {
    const r = await api<{ account?: string, folder?: string, uid?: number }>(`/api/scheduled/${s.id}`, { method: 'POST', body: { action, sendAt: sendAt?.toISOString() } })
    if (action === 'cancel') {
      mail.current = null; mail.selected = null
      toast(t('later.cancelled'), r.folder && r.uid ? { action: { label: t('common.open'), run: () => openMessage({ account: r.account!, folder: r.folder!, uid: r.uid!, seen: true } as Summary) } } : {})
    } else if (action === 'now') toast(t('later.sendingNow'))
    else toast(t('later.rescheduled', { when: whenLabel(sendAt!, true) }))
    loadMessages(false, true)
    refreshFoldersSoon(300)
    loadLater()
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}

// Pushed by the server when a scheduled letter left (or could not).
export function onScheduledEvent(info: { status?: string, subject?: string, error?: string }) {
  if (info.status === 'sent') toast(t('later.sentNow', { subject: info.subject || t('common.noSubject') }), { action: { label: t('mail.folder.sent'), run: () => setView({ role: 'sent' }) } })
  else if (info.status === 'failed') toast(t('later.failed', { subject: info.subject || t('common.noSubject'), error: info.error || '' }), { kind: 'err', ms: 12000, action: { label: t('mail.folder.scheduled'), run: () => setView({ role: 'scheduled' }) } })
  loadLater()
  refreshFoldersSoon(300)
}
