import { t, num, fmtDate, tzDayKey } from './i18n'

const AVA = ['#0B7A6E', '#B26A12', '#3D5FA8', '#8A4B9C', '#2F7D4F', '#A8433A', '#4F6475']
export const ACC_COLORS = ['var(--accent)', '#C27A1A', '#4A6FC0', '#8A4B9C', '#2F8A55', '#B5453A']

const pad = (n: number) => String(n).padStart(2, '0')

// A letter's time in the list: the time today, "Yesterday", the day this year, the full date before.
export function shortTime(iso: string): string {
  const d = new Date(iso), now = new Date()
  const k = tzDayKey(d), today = tzDayKey(now)
  if (k === today) return fmtDate(d, { hour: '2-digit', minute: '2-digit' })
  if (k === tzDayKey(new Date(now.getTime() - 86400000))) return t('date.yesterday')
  if (k.slice(0, 4) === today.slice(0, 4)) return fmtDate(d, { day: 'numeric', month: 'short' })
  return fmtDate(d, { year: 'numeric', month: '2-digit', day: '2-digit' })
}

export function longDate(iso: string): string {
  return fmtDate(iso, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function isoDay(d: Date): string { return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` }
export const ddmm = (iso: string) => iso ? fmtDate(`${iso.slice(0, 10)}T12:00:00Z`, { day: '2-digit', month: '2-digit', timeZone: 'UTC' }) : '…'

export function fileSize(b: number): string {
  if (b < 1024) return t('unit.b', { n: num(b) })
  if (b < 1024 * 1024) return t('unit.kb', { n: num(Math.round(b / 1024)) })
  if (b < 1024 * 1024 * 1024) return t('unit.mb', { n: num(b / 1024 / 1024, { maximumFractionDigits: 1 }) })
  return t('unit.gb', { n: num(b / 1024 / 1024 / 1024, { maximumFractionDigits: 1 }) })
}

export function initials(name: string): string {
  return name.replace(/[«»"'<>]/g, '').split(/[\s@.]+/).filter(Boolean).slice(0, 2).map(w => w[0]).join('').toUpperCase() || '?'
}

export function avatarColor(s: string): string {
  let h = 0
  for (const c of s) h = (h + c.charCodeAt(0)) % 997
  return AVA[h % AVA.length]
}

export const displayName = (a: { name?: string, email?: string }) => a?.name || a?.email || t('common.noSender')

export function plural(n: number, forms: [string, string, string]) {
  const m10 = n % 10, m100 = n % 100
  return forms[m10 === 1 && m100 !== 11 ? 0 : m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20) ? 1 : 2]
}
