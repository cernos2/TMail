// Admin API helpers and human-readable labels.
import { api } from './api'
import { t, num, fmtDate } from './i18n'

export const adminApi = <T = any>(path: string, opts: Record<string, any> = {}) => api<T>('/api/admin' + path, opts)

export const GB = 1024 * 1024 * 1024
const QUOTA_GB = [1, 2, 5, 10, 20, 50]
// Mailbox size presets; labels are built at render time so a language switch updates them.
export const quotas = (): { label: string, bytes: number }[] => [
  ...QUOTA_GB.map(n => ({ label: t('unit.gb', { n: num(n) }), bytes: n * GB })),
  { label: t('admin.unlimited'), bytes: 0 },
]

export function bytes(b?: number | null): string {
  if (b == null) return '—'
  if (b === 0) return '0'
  if (b < 1024 * 1024) return t('unit.kb', { n: num(Math.max(1, Math.round(b / 1024))) })
  if (b < GB) return t('unit.mb', { n: num(Math.round(b / 1024 / 1024)) })
  const d = b < 10 * GB ? 1 : 0
  return t('unit.gb', { n: num(b / GB, { minimumFractionDigits: d, maximumFractionDigits: d }) })
}

export function ago(iso?: string | null): string {
  if (!iso) return t('admin.never')
  const s = (Date.now() - new Date(iso).getTime()) / 1000
  if (s < 60) return t('admin.justNow')
  if (s < 3600) return t('admin.minutesAgo', { n: Math.floor(s / 60) })
  if (s < 86400) return t('admin.hoursAgo', { n: Math.floor(s / 3600) })
  if (s < 86400 * 30) return t('admin.daysAgo', { n: Math.floor(s / 86400) })
  return fmtDate(iso, { day: '2-digit', month: '2-digit', year: 'numeric' })
}

export function dateTime(iso: string): string {
  return fmtDate(iso, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

// Human label of an audit action code ("user.update" → admin.audit.actionUserUpdate); unknown codes are shown as is.
export function actionLabel(a: string) {
  const key = 'admin.audit.action' + a.split(/[._]/).map(w => w.charAt(0).toUpperCase() + w.slice(1)).join('')
  const label = t(key)
  return label === key ? a : label
}
