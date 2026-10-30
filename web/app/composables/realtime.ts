// Server-Sent Events from /api/events: new INBOX messages and counter changes, pushed by the
// backend's IMAP IDLE watchers. EventSource reconnects by itself; after a reconnect we resync.
import { mail, receiveNew, refreshFoldersSoon, loadMessages, openMessage, setView, type Summary } from './mail'
import { notifyNew, notifyFiled } from './notify'
import { showReminder, dropReminder, type CalEvent } from './calendar'
import { api } from './api'

let es: EventSource | null = null
let readyOnce = false
let failures = 0

export function connectEvents() {
  if (es) return
  es = new EventSource('/api/events')
  es.onmessage = (e) => {
    let ev: { type: string, account?: string, message?: Summary, event?: CalEvent }
    try { ev = JSON.parse(e.data) } catch { return }
    failures = 0
    mail.online = true
    switch (ev.type) {
      case 'ready':
        if (readyOnce) { refreshFoldersSoon(0); if (!mail.loading) loadMessages(false, true) }
        readyOnce = true
        break
      case 'new':
        if (ev.message) {
          receiveNew(ev.message)
          notifyNew(ev.message, openFromNotification)
        }
        break
      case 'filed': // auto-sorting put a new letter into an ordinary folder
        if (ev.message) { notifyFiled(ev.message, m => setView({ account: m.account, folder: m.folder })); refreshFoldersSoon(300) }
        break
      case 'reminder':
        if (ev.event) showReminder(ev.event)
        break
      case 'reminder_done': // confirmed in another tab
        if (ev.event) dropReminder(ev.event.id)
        break
      case 'counts':
        refreshFoldersSoon(500)
        break
    }
  }
  es.onerror = () => {
    mail.online = false
    if (++failures === 3) {
      // Session may have expired: /api/me redirects to the login page on 401.
      api('/api/me').catch(() => {})
    }
    // The browser gives up for good when the server answered an error during a restart (Safari
    // also after the phone slept): then the tab froze until reloaded by hand. Open a new one.
    if (es?.readyState === EventSource.CLOSED) reconnectSoon()
  }
}

let retry: ReturnType<typeof setTimeout> | undefined
function reconnectSoon() {
  if (retry) return
  retry = setTimeout(() => {
    retry = undefined
    es?.close()
    es = null
    connectEvents() // its "ready" resyncs the folders and the list
  }, Math.min(30000, 2000 * Math.max(1, failures)))
}

// Back to a tab that slept (another tab, a locked phone): check the live channel and catch up.
if (typeof document !== 'undefined') {
  let hiddenAt = 0
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') { hiddenAt = Date.now(); return }
    if (!es) return // not started (login page) or stopped on logout
    if (es.readyState === EventSource.CLOSED) reconnectSoon()
    else if (hiddenAt && Date.now() - hiddenAt > 60000) { refreshFoldersSoon(0); if (!mail.loading) loadMessages(false, true) }
  })
}

export function disconnectEvents() {
  clearTimeout(retry)
  retry = undefined
  es?.close()
  es = null
  readyOnce = false
}

function openFromNotification(m: Summary) {
  const inList = mail.messages.find(x => x.account === m.account && x.folder === m.folder && x.uid === m.uid)
  openMessage(inList || m)
}
