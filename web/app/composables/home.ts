// The home page: what needs attention across all mailboxes, the next days and the latest files.
import { reactive } from 'vue'
import { api } from './api'
import type { Attachment, Addr } from './mail'
import type { CalEvent } from './calendar'

export type HomeFile = { account: string, folder: string, uid: number, date: string, from: Addr, subject: string, sent: boolean, file: Attachment }

export const home = reactive({
  open: false,
  files: [] as HomeFile[],
  filesLoading: false,
  events: [] as CalEvent[],
  eventsLoading: false,
})

export async function loadHome(fresh = false) {
  home.filesLoading = !home.files.length
  home.eventsLoading = !home.events.length
  const from = new Date(); from.setHours(0, 0, 0, 0)
  const to = new Date(from); to.setDate(from.getDate() + 8)
  api<CalEvent[]>('/api/calendar/events', { query: { from: from.toISOString(), to: to.toISOString() } })
    .then(evs => { home.events = evs }).catch(() => {}).finally(() => { home.eventsLoading = false })
  try { home.files = await api<HomeFile[]>('/api/home/files', { query: fresh ? { fresh: '1' } : {} }) } catch {} finally { home.filesLoading = false }
}
