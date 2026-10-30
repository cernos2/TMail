// Browser notifications for new mail. Shown when the tab is hidden or not focused;
// otherwise an in-app toast is enough.
import { reactive } from 'vue'
import { toast } from './toast'
import { displayName } from './format'
import { t, fmtDate, i18n } from './i18n'
import type { Summary } from './mail'

// Letters brought in from other services carry the sender's date, and a letter may reach us a while
// after it was written; only an import of history (days and years old) is kept quiet.
const FRESH_MS = 3 * 60 * 60 * 1000
const isFresh = (m: Summary) => Date.now() - new Date(m.date).getTime() <= FRESH_MS

export const notifyState = reactive({
  supported: typeof window !== 'undefined' && 'Notification' in window,
  permission: (typeof window !== 'undefined' && 'Notification' in window ? Notification.permission : 'denied') as NotificationPermission,
  enabled: true, // new letters (user setting; permission alone is not enough)
  filed: true, // letters auto-sorted into a folder
  calendar: true, // calendar reminders as browser notifications (the card in the mail is always shown)
  bannerDismissed: false,
})

export function initNotify(s: { notify?: boolean, notifyFiled?: boolean, notifyCalendar?: boolean } = {}) {
  notifyState.enabled = s.notify !== false
  notifyState.filed = s.notifyFiled !== false
  notifyState.calendar = s.notifyCalendar !== false
  try { notifyState.bannerDismissed = localStorage.getItem('zm-notify-banner') === 'dismissed' } catch {}
}

export async function requestNotifyPermission() {
  if (!notifyState.supported) return 'denied'
  const p = await Notification.requestPermission()
  notifyState.permission = p
  if (p === 'granted') toast(t('notify.enabled'))
  else if (p === 'denied') toast(t('notify.denied'), { kind: 'err', ms: 8000 })
  return p
}

// A sample, to see how notifications look and that the browser allows them.
export async function testNotification() {
  if (!notifyState.supported) { toast(t('notify.unsupported'), { kind: 'err' }); return }
  if (notifyState.permission !== 'granted' && await requestNotifyPermission() !== 'granted') return
  try { new Notification(t('notify.testTitle'), { body: t('notify.testBody'), icon: '/icon-192.png', lang: i18n.lang }) }
  catch { toast(t('notify.testFailed'), { kind: 'err' }) }
}

export function dismissNotifyBanner() {
  notifyState.bannerDismissed = true
  try { localStorage.setItem('zm-notify-banner', 'dismissed') } catch {}
}

export function notifyNew(m: Summary, open: (m: Summary) => void) {
  if (!isFresh(m)) return
  const from = displayName(m.from)
  const subject = m.subject || t('common.noSubject')
  const background = document.visibilityState !== 'visible' || !document.hasFocus()
  if (background && notifyState.enabled && notifyState.supported && notifyState.permission === 'granted') {
    try {
      // Like Proton: a fixed title, then "From: who - subject | date time".
      const when = fmtDate(m.date, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
      const n = new Notification(t('notify.newTitle'), {
        body: t('notify.newBody', { from, subject, when }),
        icon: '/icon-192.png',
        badge: '/icon-192.png',
        tag: `${m.account}|${m.uid}`,
        lang: i18n.lang,
      })
      n.onclick = () => { window.focus(); open(m); n.close() }
      return
    } catch { /* some mobile browsers only allow notifications from a service worker */ }
  }
  if (!background) {
    toast(t('notify.newToast', { from, subject }), { action: { label: t('common.open'), run: () => open(m) }, ms: 6000 })
  }
}

// A new letter that auto-sorting has just put into an ordinary folder.
export function notifyFiled(m: Summary, open: (m: Summary) => void) {
  if (!isFresh(m)) return
  const text = t('notify.fromSubject', { from: displayName(m.from), subject: m.subject || t('common.noSubject') })
  const background = document.visibilityState !== 'visible' || !document.hasFocus()
  if (!notifyState.filed) return
  if (background && notifyState.supported && notifyState.permission === 'granted') {
    try {
      const n = new Notification(t('notify.filedTitle', { folder: m.folder }), { body: text, icon: '/icon-192.png', tag: `${m.account}|${m.folder}|${m.uid}`, lang: i18n.lang })
      n.onclick = () => { window.focus(); open(m); n.close() }
      return
    } catch {}
  }
  if (!background) toast(t('notify.filedToast', { folder: m.folder, text }), { action: { label: t('notify.openFolder'), run: () => open(m) }, ms: 6000 })
}

// Unread counter in the tab title and on the favicon.
let baseIcon: string | null = null
export function setUnreadBadge(n: number) {
  document.title = n > 0 ? t('notify.tabTitle', { n }) : 'TMAIL'
  const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) return
  baseIcon ??= link.href
  if (n <= 0) { link.href = baseIcon; link.type = 'image/svg+xml'; return }
  const c = document.createElement('canvas')
  c.width = c.height = 64
  const g = c.getContext('2d')!
  const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#0B7A6E'
  g.fillStyle = accent; g.beginPath(); g.arc(32, 32, 28, 0, Math.PI * 2); g.fill()
  g.fillStyle = '#fff'; g.font = 'bold 30px sans-serif'; g.textAlign = 'center'; g.textBaseline = 'middle'
  g.fillText(n > 99 ? '99+' : String(n), 32, 34)
  link.type = 'image/png'
  link.href = c.toDataURL('image/png')
}
