// Central webmail state: accounts, folders, the merged message list, the open message and the composer.
import { applyUserLocale, t, num, fmtDate, i18n } from './i18n'
import { crm } from './crm'
import { reactive, computed, nextTick, watch } from 'vue'
import { confirmDialog } from './dialogs'
import { applyBackground } from './backgrounds'
import { api, ApiError } from './api'
import { toast, dismissToast } from './toast'
import { linkify } from './linkify'
import { labelById } from './labels'
import { ACC_COLORS, longDate, displayName, fileSize } from './format'

export type Account = {
  address: string, name: string, canSend: boolean, personal: boolean,
  signature: string, template: string, canEditSignature: boolean
}
export type Folder = { name: string, role: string, unseen: number, total: number }
export type Addr = { name: string, email: string }
export type SentBy = { login: string, name: string }
export type Summary = {
  account: string, folder: string, uid: number, from: Addr, to: Addr[], cc?: Addr[], subject: string, date: string,
  seen: boolean, flagged: boolean, answered: boolean, hasAttachments: boolean, size: number, preview: string,
  messageId: string, sentBy?: SentBy, fresh?: boolean, dmarc?: string, attachments?: Attachment[], labels?: number[],
  inReplyTo?: string, refs?: string[]
}
export type Attachment = { part: string, name: string, mime: string, size: number, inline: boolean, preview?: '' | 'image' | 'pdf' | 'text' | 'office', risk?: '' | 'exec' | 'macro' | 'html', embedded?: boolean }
export type FullMessage = Summary & {
  html: string, remoteImages: boolean, attachments: Attachment[], replyTo: Addr[],
  inReplyTo: string, references: string, spam: boolean, auth: Record<string, string>
  av?: string[] // antivirus findings: macro | encrypted | heuristic | spam | virus
}
// A conversation in the list: its letters newest first.
export type Thread = { key: string, items: Summary[], latest: Summary }
export type ComposeMode = 'new' | 'reply' | 'replyall' | 'fwd'
// Attachments that already live on the server (forwarded message or the saved draft).
export type Carry = { account: string, folder: string, uid: number, parts: Attachment[] }
// Body is HTML from the rich editor; images pasted into it are kept in `inline` (cid → file) until saved.
export type Compose = {
  mode: ComposeMode, from: string, to: string, cc: string, bcc: string, showCc: boolean, subject: string, html: string, inline: Record<string, File>,
  files: File[], inReplyTo: string, references: string, replyFolder: string, replyUid: number,
  carry: Carry | null, sending: boolean, error: string,
  sourceAccount: string, sig: string, template: string,
  draftAccount: string, draftUid: number, savedAt: string, saveState: 'idle' | 'saving' | 'saved' | 'error', changed: number
  uploads: BigUpload[], links: BigFile[], // large files sent as links
}
export type BigFile = { token: string, url: string, thumb?: string, name: string, size: number, mime: string, expires: string }
export type BigUpload = { id: number, name: string, size: number, loaded: number, error?: string, xhr?: XMLHttpRequest }

// Names of the special folders in the user's language (getters: read at render time).
export const ROLE_NAMES: Record<string, string> = {
  get inbox() { return t('mail.folder.inbox') }, get flagged() { return t('mail.folder.flagged') },
  get drafts() { return t('mail.folder.drafts') }, get sent() { return t('mail.folder.sent') },
  get archive() { return t('mail.folder.archive') }, get junk() { return t('mail.folder.junk') },
  get trash() { return t('mail.folder.trash') },
}
export const ROLE_ICONS: Record<string, string> = {
  inbox: 'inbox', flagged: 'star', drafts: 'drafts', sent: 'sent', archive: 'archive', junk: 'junk', trash: 'trash',
}
const PAGE = 50
// Rows to show at once: a long conversation (38 letters from Ozon) is one row, so the list keeps
// fetching until it has this many rows.
const ROWS = 50

export const mail = reactive({
  me: null as null | { login: string, name: string, isAdmin: boolean, accounts: Account[], imagePolicy?: 'ask' | 'always', imageSenders?: string[], background?: string, backgroundVersion?: number,
    aiName?: string, lang?: string, timezone?: string, defaults?: { lang: string, timezone: string, country: string, org?: string } },
  folders: {} as Record<string, Folder[]>,
  view: { account: null as string | null, role: 'inbox', folder: null as string | null, label: null as number | null },
  messages: [] as Summary[],
  cursors: {} as Record<string, { before: number, hasMore: boolean }>,
  loading: false,
  listVeil: false, // a load the user asked for (folder, search, filter): the list is veiled
  loadingMore: false,
  listError: '',
  filters: { q: '', since: '', until: '', unseen: false, att: false, sort: 'new' as 'new' | 'old' | 'from' | 'unread' },
  selected: null as string | null,
  current: null as FullMessage | null,
  // The open conversation: all its letters (oldest first, from Inbox/Sent/Archive), the earlier
  // ones the user expanded, and the list rows the toolbar actions apply to.
  thread: [] as Summary[],
  threadOpen: {} as Record<string, FullMessage | 'loading' | 'error'>,
  threadAll: false,
  threadItems: [] as Summary[],
  currentLoading: false,
  currentError: '',
  showImages: false,
  originalColors: false, // dark theme: show the message in its own colours
  compose: null as Compose | null,
  composeModal: false, // true: separate compose window; false: reply inside the reader
  composeView: 'open' as 'open' | 'min' | 'max',
  composePos: null as null | { x: number, y: number }, // dragged compose window; null = docked bottom right
  reading: false,
  identities: [] as Identity[],
  settingsTab: '', // set to open the settings window on a tab from anywhere
  sideOpen: false,
  online: true,
  searchPartial: false, // the letters' text is still being searched on the server
  searchIndexed: false, // the quick hits came from the index: only attachments are left
  defaultFrom: '', // the user's choice for new letters ('' = the mailbox that is open)
  threadNew: null as Summary | null, // a letter that just came into the open conversation
  threadFull: {} as Record<string, Summary[]>, // list row key → the whole conversation (with our replies from Sent)
})

export const keyOf = (m: { account: string, folder: string, uid: number }) => `${m.account}|${m.folder}|${m.uid}`

export function accountColor(addr: string) {
  const i = mail.me?.accounts.findIndex(a => a.address === addr) ?? 0
  return ACC_COLORS[Math.max(0, i) % ACC_COLORS.length]
}
export const accountOf = (addr: string) => mail.me?.accounts.find(a => a.address === addr)

export const viewTitle = computed(() => {
  if (mail.view.label) return labelById(mail.view.label)?.name || t('mail.label')
  if (mail.view.folder) return mail.view.folder
  const r = ROLE_NAMES[mail.view.role] || t('mail.mail')
  return mail.view.account ? `${r} · ${mail.view.account}` : r
})

export const unreadInbox = computed(() =>
  Object.values(mail.folders).reduce((n, fs) => n + (fs.find(f => f.role === 'inbox')?.unseen || 0), 0))

// All letters of a folder kind (Inbox, Spam…) in one mailbox or across all of them.
export function totalFor(role: string, account?: string) {
  if (role === 'flagged') return 0
  const accs = account ? [account] : Object.keys(mail.folders)
  return accs.reduce((n, a) => n + (mail.folders[a]?.find(f => f.role === role)?.total || 0), 0)
}
export const fmtCount = (n: number) => num(n)

export function unseenFor(role: string, account?: string) {
  if (role === 'flagged') return 0
  const accs = account ? [account] : Object.keys(mail.folders)
  return accs.reduce((n, a) => n + (mail.folders[a]?.find(f => f.role === role)?.unseen || 0), 0)
}

export function resetFilters() {
  Object.assign(mail.filters, { q: '', since: '', until: '', unseen: false, att: false })
  loadMessages()
}
// How many letters the open folder holds (from the folder counters), to explain an empty filtered list.
export function viewTotal(): number {
  const v = mail.view
  const accs = v.account ? [v.account] : Object.keys(mail.folders)
  return accs.reduce((n, a) => n + ((mail.folders[a] || []).find(f => v.folder ? f.name === v.folder : f.role === v.role)?.total || 0), 0)
}
export const filtersActive = computed(() => !!(mail.filters.q || mail.filters.since || mail.filters.until || mail.filters.unseen))

export const visibleMessages = computed(() => {
  let list = mail.filters.att ? mail.messages.filter(m => m.hasAttachments) : mail.messages.slice()
  const by: Record<string, (a: Summary, b: Summary) => number> = {
    new: (a, b) => b.date.localeCompare(a.date),
    old: (a, b) => a.date.localeCompare(b.date),
    from: (a, b) => displayName(a.from).localeCompare(displayName(b.from), i18n.lang) || b.date.localeCompare(a.date),
    unread: (a, b) => (Number(a.seen) - Number(b.seen)) || b.date.localeCompare(a.date),
  }
  return list.sort(by[mail.filters.sort])
})

// ---------- conversations ----------

// Reply/forward prefixes of subjects, as mail programs write them in English, German and Russian
// (detection only, not interface text).
// Spam tags put before the subject by other servers ("***SPAM*** Re: Order") count as prefixes too.
const RE_PREFIX = /^\s*(((re|fwd?|aw|wg|отв|пересл|ответ)(\[\d+\])?\s*:|\*+\s*spam\s*\*+|\[spam\])\s*)+/i
export const baseSubject = (s: string) => (s || '').replace(RE_PREFIX, '').replace(/\s+/g, ' ').trim().toLowerCase()
// Like Gmail: a letter joins a conversation when it refers to one of its letters (Message-ID,
// In-Reply-To, References) AND has the same subject without Re:/Fwd:. The subject alone never joins
// (a client's every new "Request" is its own conversation), and a changed subject starts a new one.
export const visibleThreads = computed<Thread[]>(() => {
  const list = visibleMessages.value
  if (mail.view.role === 'drafts' && !mail.view.folder) return list.map(m => ({ key: keyOf(m), items: [m], latest: m }))
  const parent = list.map((_, i) => i)
  const find = (i: number): number => { while (parent[i] !== i) { parent[i] = parent[parent[i]]; i = parent[i] } return i }
  const union = (a: number, b: number) => { const x = find(a), y = find(b); if (x !== y) parent[Math.max(x, y)] = Math.min(x, y) }
  const byId = new Map<string, number>()
  list.forEach((m, i) => {
    const subj = baseSubject(m.subject)
    for (const id of [m.messageId, m.inReplyTo, ...(m.refs || [])]) {
      if (!id) continue
      const k = `${m.account}|${subj}|${id}`
      const j = byId.get(k)
      if (j === undefined) byId.set(k, i)
      else union(i, j)
    }
  })
  const groups = new Map<number, Summary[]>()
  list.forEach((m, i) => { const r = find(i); groups.set(r, [...(groups.get(r) || []), m]) })
  // Keep the list's sort order: a conversation sits where its first letter in that order sits.
  return [...groups.values()].map(items => {
    const byDate = items.slice().sort((a, b) => b.date.localeCompare(a.date) || (a.folder === b.folder ? b.uid - a.uid : 0))
    return { key: keyOf(byDate[byDate.length - 1]), items: byDate, latest: byDate[0] }
  })
})

export const threadOf = (m: Summary) => visibleThreads.value.find(th => th.items.some(x => keyOf(x) === keyOf(m)))

// Conversations asked for ahead (the pointer rested on the row): the click finds them ready, for a
// minute; one search at a time, each one reads the headers of several folders on the server.
const threadCache = new Map<string, { at: number, p: Promise<{ messages: Summary[] }> }>()
let threadPrefetching = 0
function fetchThread(m: Summary) {
  const k = keyOf(m)
  const hit = threadCache.get(k)
  if (hit && Date.now() - hit.at < 60000) return hit.p
  const p = api<{ messages: Summary[] }>('/api/thread', { query: { account: m.account, folder: m.folder, uid: m.uid } })
  threadCache.set(k, { at: Date.now(), p })
  p.catch(() => threadCache.delete(k))
  if (threadCache.size > 30) threadCache.delete(threadCache.keys().next().value!)
  return p
}
function prefetchThread(m: Summary) {
  const hit = threadCache.get(keyOf(m))
  if ((hit && Date.now() - hit.at < 60000) || threadPrefetching) return
  threadPrefetching++
  fetchThread(m).then(r => {
    // The conversation opens on its newest letter: that one is fetched ahead too.
    const newest = r.messages[r.messages.length - 1]
    if (newest && keyOf(newest) !== keyOf(m)) prefetchMessage(newest)
  }).catch(() => {}).finally(() => threadPrefetching--)
}

async function loadThread(m: Summary, seq: number, expandNewest = true, fresh = false) {
  try {
    if (fresh) threadCache.delete(keyOf(m))
    const r = await fetchThread(m)
    if (seq !== openSeq) return
    mail.thread = r.messages
    // The list row counts the whole conversation, our replies included (like Gmail).
    const row = threadOf(m)
    if (row && r.messages.length > 1) mail.threadFull[row.key] = r.messages
    // Like Gmail: the conversation opens on its newest letter (often our own reply), the earlier
    // ones — the clicked one too — are folded above it.
    const newest = r.messages[r.messages.length - 1]
    if (expandNewest && newest && keyOf(newest) !== keyOf(m) && new Date(newest.date).getTime() > new Date(m.date).getTime()) {
      openMessage(newest, mail.threadItems.slice())
    }
  } catch { /* the letter itself is shown anyway */ }
}
// After our reply went out: the conversation (and its list row) shows it.
export function refreshThread() {
  if (mail.current) loadThread(mail.current, openSeq, true, true)
}

export async function expandInThread(m: Summary) {
  const k = keyOf(m)
  const cur = mail.threadOpen[k]
  if (cur && cur !== 'error') { delete mail.threadOpen[k]; return }
  mail.threadOpen[k] = 'loading'
  try {
    mail.threadOpen[k] = await api<FullMessage>('/api/message', { query: { account: m.account, folder: m.folder, uid: m.uid } })
    m.seen = true
  } catch {
    mail.threadOpen[k] = 'error'
  }
}

export const hasMore = computed(() => Object.values(mail.cursors).some(c => c.hasMore))

// ---------- loading ----------

export async function loadMe() {
  mail.me = await api('/api/me')
  await applyUserLocale(mail.me)
  applyBackground(mail.me!.background || '', mail.me!.backgroundVersion || 0)
  // Needed only to write a letter: the list does not wait for it.
  api<Identity[]>('/api/identities').then(v => { mail.identities = v }).catch(() => { mail.identities = [] })
}

// External addresses connected in the mail collector that can be sent from (via their own SMTP).
export type Identity = { address: string, name: string, mailbox: string }
export const identityOf = (addr: string) => mail.identities.find(i => i.address === addr.toLowerCase())

let foldersTimer: ReturnType<typeof setTimeout> | null = null
export function refreshFoldersSoon(ms = 600) {
  if (foldersTimer) clearTimeout(foldersTimer)
  foldersTimer = setTimeout(() => { loadFolders().catch(() => {}) }, ms)
}

export async function loadFolders() {
  const res = await api<{ accounts: { account: string, folders: Folder[] | null, error?: string }[] }>('/api/folders')
  const next: Record<string, Folder[]> = {}
  for (const a of res.accounts) next[a.account] = a.folders || mail.folders[a.account] || []
  mail.folders = next
}

function scope(): string[] {
  return mail.view.account ? [mail.view.account] : (mail.me?.accounts.map(a => a.address) || [])
}

function queryFor(account: string, before = 0, limit = PAGE) {
  const q: Record<string, string | number> = { account, limit }
  if (mail.view.label) q.label = mail.view.label
  if (mail.view.folder) q.folder = mail.view.folder
  else q.role = mail.view.role === 'flagged' ? 'inbox' : mail.view.role
  if (mail.view.role === 'flagged' && !mail.view.folder) q.flagged = '1'
  if (before) q.before = before
  if (mail.filters.q.trim()) q.q = mail.filters.q.trim()
  if (mail.filters.since) q.since = mail.filters.since
  if (mail.filters.until) q.until = mail.filters.until
  if (mail.filters.unseen) q.unseen = '1'
  return q
}

let loadSeq = 0
let partialTimer: ReturnType<typeof setTimeout> | undefined
let partialTries = 0
// quiet: a refresh in the background (new mail, back to the tab) — the list stays usable, unveiled.
export async function loadMessages(again = false, quiet = false) {
  if (!again) partialTries = 0
  const seq = ++loadSeq
  const accs = scope()
  mail.loading = true
  mail.listVeil = !again && !quiet
  mail.listError = ''
  const results = await Promise.allSettled(accs.map(a =>
    api<{ messages: Summary[], hasMore: boolean, partial?: boolean, indexed?: boolean }>('/api/messages', { query: queryFor(a) })))
  if (seq !== loadSeq) return
  // A long text search goes on on the server: the subject/address hits now, the rest a bit later.
  mail.searchPartial = results.some(r => r.status === 'fulfilled' && r.value.partial)
  mail.searchIndexed = results.every(r => r.status === 'fulfilled' && r.value.indexed)
  clearTimeout(partialTimer)
  if (mail.searchPartial && partialTries++ < 40) partialTimer = setTimeout(() => { if (seq === loadSeq) loadMessages(true) }, 4000)
  const msgs: Summary[] = []
  const cursors: typeof mail.cursors = {}
  const failed: string[] = []
  results.forEach((r, i) => {
    const acc = accs[i]
    if (r.status === 'fulfilled') {
      msgs.push(...r.value.messages)
      const uids = r.value.messages.map(m => m.uid)
      cursors[acc] = { before: uids.length ? Math.min(...uids) : 0, hasMore: r.value.hasMore }
    } else {
      failed.push(acc)
    }
  })
  mail.messages = msgs
  mail.cursors = cursors
  mail.loading = false
  mail.listVeil = false
  if (visibleThreads.value.length < ROWS && hasMore.value) loadMore(ROWS)
  if (failed.length === accs.length && accs.length) {
    mail.listError = (results[0] as PromiseRejectedResult).reason?.message || t('mail.loadFailed')
  } else if (failed.length) {
    toast(t('mail.loadFailedFor', { accounts: failed.join(', ') }), { kind: 'err' })
  }
}

// Next page for the endless list: fetches until `rows` more rows appear (or the folder ends).
export async function loadMore(rows = ROWS) {
  if (mail.loadingMore) return
  mail.loadingMore = true
  const seq = loadSeq
  const want = visibleThreads.value.length + rows
  try {
    for (let round = 0; round < 8 && seq === loadSeq; round++) {
      const accs = Object.entries(mail.cursors).filter(([, c]) => c.hasMore && c.before > 0).map(([a]) => a)
      if (!accs.length || await loadPage(accs, seq) === false || visibleThreads.value.length >= want) break
    }
  } catch (e: any) {
    toast(e.message, { kind: 'err' })
  } finally {
    mail.loadingMore = false
  }
}

async function loadPage(accs: string[], seq: number) {
  {
    const res = await Promise.all(accs.map(a =>
      api<{ messages: Summary[], hasMore: boolean }>('/api/messages', { query: queryFor(a, mail.cursors[a].before, 200) })))
    if (seq !== loadSeq) return false
    res.forEach((r, i) => {
      const acc = accs[i]
      const have = new Set(mail.messages.map(keyOf))
      mail.messages.push(...r.messages.filter(m => !have.has(keyOf(m))))
      const uids = r.messages.map(m => m.uid)
      mail.cursors[acc] = { before: uids.length ? Math.min(...uids) : mail.cursors[acc].before, hasMore: r.hasMore && uids.length > 0 }
    })
    return true
  }
}

export function setView(v: { account?: string | null, role?: string, folder?: string | null, label?: number | null }) {
  if (mail.compose && !mail.composeModal) stashInlineReply()
  mail.view = { account: v.account ?? null, role: v.role ?? 'inbox', folder: v.folder ?? null, label: v.label ?? null }
  // Another folder starts unfiltered: a forgotten "Unread" filter made full folders look empty.
  Object.assign(mail.filters, { q: '', since: '', until: '', unseen: false, att: false })
  mail.selected = null
  mail.current = null
  mail.reading = false
  mail.sideOpen = false
  crm.open = false // a folder click leaves «Contacts» for the mail
  loadMessages()
}

// ---------- open / act ----------

let openSeq = 0
// Letters fetched before they are clicked (the pointer rests on the row ~0.15 s): opening is then
// instant even when the server is busy. Fetched with "peek" — not marked read until opened.
const fullCache = new Map<string, FullMessage>()
const prefetching = new Map<string, Promise<FullMessage>>()
export function prefetchMessage(m: Summary) {
  const k = keyOf(m)
  if (fullCache.has(k) || prefetching.has(k) || prefetching.size >= 2 || m.size > 3 << 20) return
  const p = api<FullMessage>('/api/message', { query: { account: m.account, folder: m.folder, uid: m.uid, peek: '1' } })
  prefetching.set(k, p)
  p.then(full => {
    fullCache.set(k, full)
    if (fullCache.size > 30) fullCache.delete(fullCache.keys().next().value!)
  }).catch(() => {}).finally(() => prefetching.delete(k))
}
export const forgetMessage = (m: Summary) => fullCache.delete(keyOf(m))
// The pointer rests on a list row: its letter and its conversation are fetched ahead.
export function prefetchRow(m: Summary) {
  prefetchThread(m)
  prefetchMessage(m)
}

export async function openMessage(m: Summary, keepItems?: Summary[]) {
  if (mail.compose && !mail.composeModal) stashInlineReply()
  mail.threadNew = null
  const seq = ++openSeq
  mail.selected = keyOf(m)
  mail.reading = true
  mail.current = null
  mail.currentError = ''
  mail.currentLoading = true
  mail.thread = []
  mail.threadOpen = {}
  mail.threadAll = false
  const th = threadOf(m)
  // The toolbar (archive, delete…) acts on the conversation's rows in the list, also when it was
  // opened on our reply from Sent.
  mail.threadItems = keepItems || (th ? th.items.slice() : [m])
  loadThread(m, seq)
  // Opening a conversation marks all of it as read, like Gmail.
  const unreadOthers = mail.threadItems.filter(x => !x.seen && keyOf(x) !== keyOf(m))
  if (unreadOthers.length) act('seen', unreadOthers)
  mail.showImages = false
  mail.originalColors = false
  m.fresh = false
  try {
    // Fetched ahead when the pointer rested on the row (see prefetchMessage): shown at once.
    const k = keyOf(m)
    const early = fullCache.get(k) || await prefetching.get(k)?.catch(() => undefined)
    const full = early || await api<FullMessage>('/api/message', { query: { account: m.account, folder: m.folder, uid: m.uid } })
    if (seq !== openSeq) return
    mail.current = early ? { ...full, flagged: m.flagged, labels: m.labels ?? full.labels } : full
    restoreParked(mail.current, seq)
    if (!m.seen) {
      m.seen = true
      const f = mail.folders[m.account]?.find(x => x.name === m.folder)
      if (f && f.unseen > 0) f.unseen--
      if (early) act('seen', [m]) // fetched without marking it read
    }
  } catch (e: any) {
    if (seq === openSeq) mail.currentError = e.message
  } finally {
    if (seq === openSeq) mail.currentLoading = false
  }
}

// Close the open letter (the list stays as it was).
export function closeMessage() {
  if (mail.compose && !mail.composeModal) stashInlineReply()
  openSeq++
  Object.assign(mail, { current: null, selected: null, reading: false, currentError: '', currentLoading: false, thread: [], threadOpen: {}, threadItems: [], threadNew: null })
}

// Message keys of the toast after a move, by target folder.
const ACTION_DONE: Record<string, string> = {
  archive: 'mail.movedArchive', junk: 'mail.movedJunk', inbox: 'mail.movedInbox', trash: 'mail.movedTrash',
}

export async function act(action: 'seen' | 'unseen' | 'flag' | 'unflag' | 'move' | 'delete' | 'label' | 'unlabel', items: Summary[], target?: string) {
  if (!items.length) return
  const groups = new Map<string, Summary[]>()
  for (const m of items) {
    const k = `${m.account}|${m.folder}`
    groups.set(k, [...(groups.get(k) || []), m])
  }
  const removes = action === 'move' || action === 'delete'
  const keys = new Set(items.map(keyOf))
  // Optimistic update
  for (const m of items) {
    if (action === 'seen') m.seen = true
    if (action === 'unseen') m.seen = false
    if (action === 'flag') m.flagged = true
    if (action === 'unflag') m.flagged = false
    if (action === 'label' && !m.labels?.includes(Number(target))) m.labels = [...(m.labels || []), Number(target)]
    if (action === 'unlabel') m.labels = (m.labels || []).filter(x => x !== Number(target))
  }
  if (mail.current && keys.has(keyOf(mail.current)) && (action === 'label' || action === 'unlabel')) {
    const cur = mail.current
    cur.labels = action === 'label' ? [...new Set([...(cur.labels || []), Number(target)])] : (cur.labels || []).filter(x => x !== Number(target))
  }
  if (mail.current && keys.has(keyOf(mail.current))) {
    if (action === 'flag' || action === 'unflag') mail.current.flagged = action === 'flag'
    if (action === 'seen' || action === 'unseen') mail.current.seen = action === 'seen'
  }
  // The open letter removed: the pane goes back to the calendar. The next letter is NOT opened by
  // itself — it would be marked read without being looked at.
  let closedOpen = false
  if (removes) {
    mail.messages = mail.messages.filter(m => !keys.has(keyOf(m)))
    mail.threadItems = mail.threadItems.filter(m => !keys.has(keyOf(m)))
    if (mail.current && keys.has(keyOf(mail.current))) {
      closedOpen = true
      mail.current = null
      mail.selected = null
      if (mail.compose && !mail.composeModal) stashInlineReply()
    }
  }
  try {
    await Promise.all([...groups.values()].map(g => api('/api/messages/action', {
      method: 'POST',
      body: { account: g[0].account, folder: g[0].folder, uids: g.map(m => m.uid), action, target },
    })))
    if (removes) {
      const inTrash = items[0].folder === (mail.folders[items[0].account]?.find(f => f.role === 'trash')?.name)
      toast(action === 'delete' ? (inTrash ? t('mail.deletedForever') : t('mail.movedTrash')) : t(ACTION_DONE[target || ''] || 'mail.moved'))
      if (closedOpen) mail.reading = false
    }
    refreshFoldersSoon(300)
  } catch (e: any) {
    toast(e.message, { kind: 'err' })
    loadMessages()
  }
}

// ---------- compose ----------

export function htmlToText(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('style,script,head').forEach(n => n.remove())
  doc.querySelectorAll('img').forEach(i => i.replaceWith(i.getAttribute('alt') ? `[${i.getAttribute('alt')}]` : ''))
  doc.querySelectorAll('br').forEach(br => br.replaceWith('\n'))
  doc.querySelectorAll('p,div,tr,li,h1,h2,h3,h4').forEach(el => el.append('\n'))
  // Quoted text gets "> " in front of every line (innermost first).
  Array.from(doc.querySelectorAll('blockquote')).reverse().forEach(q => {
    const txt = (q.textContent || '').replace(/\n+$/, '')
    q.replaceWith('\n' + txt.split('\n').map(l => '> ' + l).join('\n') + '\n')
  })
  return (doc.body.textContent || '').replace(/ /g, ' ').replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim()
}

const escHtml = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]!))
export const textHtml = (s: string) => escHtml(s).replace(/\n/g, '<br>')
// Signatures made in the editor are HTML (links, pictures); older ones are plain text.
export const isHtmlSig = (s: string) => /<(div|p|br|a|img|b|strong|i|em|u|span|font|table|ul|ol)\b/i.test(s)
const sigInner = (block: string) => isHtmlSig(block) ? block : textHtml(block)
const sigHtml = (block: string) => block ? `<div data-tmail-sig="">${sigInner(block)}</div>` : ''
// The text of a signature, for a one-line look at it.
export const sigText = (s: string) => isHtmlSig(s) ? new DOMParser().parseFromString(s, 'text/html').body.innerText || '' : s

const fmtAddr = (a: Addr) => a.name ? `${a.name} <${a.email}>` : a.email
const prefixed = (p: string, s: string) => new RegExp(`^\\s*${p}:`, 'i').test(s) ? s : `${p}: ${s}`

// Signature with {name} {login} {email} filled in, as a "-- " block.
export function signatureBlock(addr: string): string {
  const a = accountOf(addr) || accountOf(identityOf(addr)?.mailbox || '')
  if (!a?.signature?.trim()) return ''
  const html = isHtmlSig(a.signature)
  const v = (s: string) => html ? escHtml(s) : s
  const sig = a.signature
    .replace(/\{name\}/g, v(mail.me?.name || mail.me?.login || ''))
    .replace(/\{login\}/g, v(mail.me?.login || ''))
    .replace(/\{email\}/g, v(addr))
  // The signature may bring its own «--» line.
  if (html) return /^\s*--/.test(sigText(sig)) ? sig : `<div>--&nbsp;</div>${sig}`
  return /^\s*--\s*(\n|$)/.test(sig) ? sig : `-- \n${sig}`
}

// Swap the signature when the sender address changes.
export function switchSender(c: Compose, from: string) {
  const next = signatureBlock(from)
  const doc = new DOMParser().parseFromString(c.html, 'text/html')
  const old = doc.querySelector('[data-tmail-sig]')
  if (old) {
    if (next) old.innerHTML = sigInner(next)
    else old.remove()
  } else if (next) {
    const div = doc.createElement('div')
    div.setAttribute('data-tmail-sig', '')
    div.innerHTML = sigInner(next)
    const anchor = doc.querySelector('[data-tmail-quote],[data-tmail-fwd]')
    if (anchor) anchor.before(div)
    else doc.body.append(div)
  }
  c.html = doc.body.innerHTML
  c.sig = next
}

function senderAccount(preferred?: string) {
  const accs = mail.me?.accounts || []
  return accs.find(a => a.address === preferred && a.canSend)?.address || accs.find(a => a.canSend)?.address || ''
}

function emptyCompose(mode: ComposeMode): Compose {
  return {
    mode, from: senderAccount(mail.defaultFrom || mail.view.account || undefined), to: '', cc: '', bcc: '', showCc: false, subject: '', html: '', inline: {},
    files: [], inReplyTo: '', references: '', replyFolder: '', replyUid: 0, carry: null, sending: false, error: '', sourceAccount: '',
    sig: '', template: '', draftAccount: '', draftUid: 0, savedAt: '', saveState: 'idle', changed: 0, uploads: [], links: [],
  }
}

export async function startCompose(mode: ComposeMode, opts: { modal?: boolean } = {}) {
  // One composer at a time: the current one goes to Drafts.
  if (mail.compose) await closeCompose()
  const m = mail.current
  const base = emptyCompose(mode)
  if (mode !== 'new' && m) {
    base.from = senderAccount(m.account)
    const cameTo = [...m.to, ...(m.cc || [])].map(a => a.email.toLowerCase())
    const ext = mail.identities.find(i => i.mailbox === m.account && cameTo.includes(i.address))
    if (ext) base.from = ext.address
    base.sourceAccount = m.account
    const body = htmlToText(m.html)
    if (mode === 'fwd') {
      base.subject = prefixed('Fwd', m.subject)
      // The forward header goes into the letter in the writer's interface language.
      base.html = `<div data-tmail-fwd=""><br>${escHtml(t('mail.fwd.head'))}<br>${escHtml(t('mail.fwd.from', { from: fmtAddr(m.from) }))}`
        + `<br>${escHtml(t('mail.fwd.date', { date: longDate(m.date) }))}<br>${escHtml(t('mail.fwd.subject', { subject: m.subject }))}`
        + `<br>${escHtml(t('mail.fwd.to', { to: m.to.map(fmtAddr).join(', ') }))}<br><br>${textHtml(body)}</div>`
      if (m.attachments.length) base.carry = { account: m.account, folder: m.folder, uid: m.uid, parts: m.attachments.slice() }
    } else {
      const own = new Set((mail.me?.accounts || []).map(a => a.address))
      const target = (m.replyTo[0] || m.from)
      const isOwn = own.has(m.from.email)
      // Replying to a message we sent ourselves goes to its original recipients.
      const primary = isOwn ? m.to : [target]
      base.to = primary.map(fmtAddr).join(', ')
      if (mode === 'replyall') {
        const seen = new Set([...primary.map(a => a.email), base.from])
        const rest = [...m.to, ...(m.cc || [])].filter(a => !seen.has(a.email) && !own.has(a.email))
        base.cc = rest.map(fmtAddr).join(', ')
        base.showCc = rest.length > 0
      }
      base.subject = prefixed('Re', m.subject)
      base.inReplyTo = m.messageId
      base.references = m.references || ''
      base.replyFolder = m.folder
      base.replyUid = m.uid
      base.html = `<div data-tmail-quote=""><br><div>${escHtml(t('mail.quoteHead', { date: longDate(m.date), from: fmtAddr(m.from) }))}</div><blockquote>${textHtml(body)}</blockquote></div>`
    }
  }
  // The mailbox's start template («Hello,») and signature — in replies and forwards too.
  base.sig = signatureBlock(base.from)
  base.template = (accountOf(base.from) || accountOf(identityOf(base.from)?.mailbox || ''))?.template || ''
  const top = `<div>${base.template ? textHtml(base.template) : '<br>'}</div>` + (base.sig ? `<div><br></div>${sigHtml(base.sig)}` : '')
  base.html = mode === 'new' || !m ? top : top + base.html
  mail.composeModal = !!opts.modal || mode === 'new' || !m
  mail.composeView = 'open'
  mail.compose = base
}

// composeTo: a new letter to an address clicked in a letter (mailto: links and plain addresses).
export async function composeTo(p: { to: string, cc?: string, subject?: string, body?: string }) {
  await startCompose('new', { modal: true })
  const c = mail.compose
  if (!c) return
  c.to = p.to
  if (p.cc) { c.cc = p.cc; c.showCc = true }
  if (p.subject) c.subject = p.subject
  if (p.body) c.html = `<div>${textHtml(p.body)}</div>` + c.html
}

// True when the user has typed something beyond the inserted template, signature and quote.
export function composeDirty(c: Compose | null = mail.compose) {
  if (!c) return false
  if (c.files.length || Object.keys(c.inline).length || (c.carry?.parts.length && c.mode !== 'fwd')) return true
  const doc = new DOMParser().parseFromString(c.html, 'text/html')
  doc.querySelectorAll('[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd]').forEach(n => n.remove())
  if (doc.querySelector('img')) return true
  let own = (doc.body.textContent || '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim()
  const tpl = c.template.replace(/\s+/g, ' ').trim()
  if (tpl && own.startsWith(tpl)) own = own.slice(tpl.length).trim()
  if (own) return true
  return c.mode === 'new' ? !!(c.subject.trim() || c.to.trim() || c.cc.trim() || c.bcc.trim()) : (c.mode === 'fwd' && !!c.to.trim())
}

// Body as it is sent: inline images point to cid:, editor-only markup is dropped.
function outgoing(c: Compose) {
  const doc = new DOMParser().parseFromString(c.html, 'text/html')
  linkify(doc) // addresses and sites typed as text go out as links
  const cids: string[] = []
  doc.querySelectorAll('img').forEach(img => {
    const cid = img.getAttribute('data-cid')
    if (cid) { img.setAttribute('src', 'cid:' + cid); img.removeAttribute('data-cid'); cids.push(cid) }
    else if (/^(blob:|\/api\/)/.test(img.getAttribute('src') || '')) img.remove()
  })
  return { html: doc.body.innerHTML, text: htmlToText(c.html), cids }
}

function composeForm(c: Compose, files: File[]) {
  const body = outgoing(c)
  const fd = new FormData()
  fd.set('from', c.from); fd.set('to', c.to); fd.set('cc', c.cc); fd.set('bcc', c.bcc)
  fd.set('subject', c.subject); fd.set('html', body.html); fd.set('text', body.text)
  if (c.inReplyTo) { fd.set('inReplyTo', c.inReplyTo); fd.set('references', c.references) }
  if (c.replyUid) { fd.set('replyFolder', c.replyFolder); fd.set('replyUid', String(c.replyUid)) }
  // Images still in the browser are uploaded; the rest already live in the saved draft (carry).
  const uploaded: string[] = []
  const serverCids: string[] = []
  for (const cid of body.cids) {
    const f = c.inline[cid]
    if (f) { fd.append('inline', f, f.name || 'image.png'); fd.append('inlineCid', cid); uploaded.push(cid) }
    else serverCids.push(cid)
  }
  if (c.carry && (c.carry.parts.length || serverCids.length)) {
    fd.set('carryAccount', c.carry.account); fd.set('carryFolder', c.carry.folder); fd.set('carryUid', String(c.carry.uid))
    fd.set('carryParts', c.carry.parts.map(p => p.part).join(','))
    fd.set('carryCids', serverCids.join(','))
  }
  if (c.draftUid) { fd.set('draftAccount', c.draftAccount); fd.set('draftUid', String(c.draftUid)) }
  for (const f of files) fd.append('files', f, f.name)
  return { fd, uploaded }
}

// Registers an image pasted into the body; returns its Content-ID and a preview URL.
export function addInlineImage(f: File) {
  const c = mail.compose!
  const cid = `${Date.now().toString(36)}.${Math.random().toString(36).slice(2, 10)}@tmail`
  c.inline[cid] = f
  return { cid, url: URL.createObjectURL(f) }
}

// ---------- drafts ----------

const saving = new WeakMap<Compose, Promise<boolean>>()

// saveDraft stores the composer in the Drafts folder (replacing the previous version).
export async function saveDraft(c: Compose | null = mail.compose): Promise<boolean> {
  if (!c || c.sending || (!composeDirty(c) && !c.draftUid)) return false
  const prev = saving.get(c)
  if (prev) await prev
  const files = c.files.slice()
  const changedAt = c.changed
  c.saveState = 'saving'
  const run = (async () => {
    try {
      const { fd, uploaded } = composeForm(c, files)
      const res = await api<{ account: string, folder: string, uid: number, attachments: Attachment[], savedAt: string }>(
        '/api/drafts', { method: 'POST', body: fd })
      c.draftAccount = res.account
      c.draftUid = res.uid
      // The saved draft now holds the attachments and the pasted images; later saves copy them from it.
      c.carry = { account: res.account, folder: res.folder, uid: res.uid, parts: res.attachments }
      c.files = c.files.filter(f => !files.includes(f))
      for (const cid of uploaded) delete c.inline[cid]
      c.savedAt = res.savedAt
      c.saveState = c.changed === changedAt ? 'saved' : 'idle'
      refreshFoldersSoon(800)
      return true
    } catch (e: any) {
      c.saveState = 'error'
      return false
    }
  })()
  saving.set(c, run)
  const ok = await run
  saving.delete(c)
  return ok
}

// Autosave a few seconds after the user stops typing.
export function watchCompose() {
  let timer: ReturnType<typeof setTimeout> | null = null
  // Compare content, not objects: saving replaces the files/carry arrays without changing what the user wrote.
  watch(() => {
    const c = mail.compose
    // Attachments are compared by name wherever they live (the server reports encoded sizes), so moving a file from the browser
    // into the saved draft is not a change.
    if (!c) return null
    const atts = [...c.files.map(f => f.name), ...(c.carry?.parts.map(x => x.name) || [])].sort()
    return c ? JSON.stringify([c.from, c.to, c.cc, c.bcc, c.subject, c.html, atts]) : null
  }, (now, before) => {
    const c = mail.compose
    if (!c || !now || !before) return
    c.changed++
    if (c.saveState === 'saved') c.saveState = 'idle'
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => { if (mail.compose === c) saveDraft(c) }, 3000)
  })
}

function draftsShown() {
  if (mail.view.role === 'drafts' && !mail.view.folder) loadMessages(false, true)
}

// Closing never loses text: anything written is kept in Drafts.
export async function closeCompose() {
  const c = mail.compose
  if (!c) return
  mail.compose = null
  mail.composeModal = false
  mail.composeView = 'open'
  if (composeDirty(c) || c.draftUid) {
    const ok = await saveDraft(c)
    toast(ok ? t('mail.draftSaved') : t('mail.draftSaveFailed'), { kind: ok ? 'info' : 'err',
      action: ok ? { label: t('common.open'), run: () => openDraftRef({ account: c.draftAccount, uid: c.draftUid }) } : undefined })
    draftsShown()
  }
}

// ---------- unfinished inline replies ----------
// Switching to another letter parks the reply: it is saved to Drafts and remembered for its letter
// (in memory and in localStorage), so coming back to the letter brings the reply back in place.
const parked = new Map<string, Compose>()
const PARK_KEY = 'tmail.parkedReplies'
const replyKey = (c: Compose) => c.replyUid ? `${c.sourceAccount || c.draftAccount || c.from}|${c.replyFolder}|${c.replyUid}` : ''
function parkedStore(): Record<string, { account: string, uid: number }> {
  try { return JSON.parse(localStorage.getItem(PARK_KEY) || '{}') } catch { return {} }
}
function setParked(key: string, v: { account: string, uid: number } | null) {
  try {
    const all = parkedStore()
    if (v) all[key] = v
    else delete all[key]
    localStorage.setItem(PARK_KEY, JSON.stringify(all))
  } catch {}
}
export function forgetParked(c: Compose | null) {
  const k = c && replyKey(c)
  if (!k) return
  parked.delete(k)
  setParked(k, null)
}

function stashInlineReply() {
  const c = mail.compose
  if (!c) return
  mail.compose = null
  if (!composeDirty(c) && !c.draftUid) return
  const k = replyKey(c)
  if (k) parked.set(k, c)
  saveDraft(c).then(ok => {
    if (ok) {
      if (k) setParked(k, { account: c.draftAccount, uid: c.draftUid })
      toast(t('mail.replyParked'))
      draftsShown()
    } else {
      toast(t('mail.replyParkFailed'), { kind: 'err', ms: 10000, action: { label: t('mail.restore'), run: () => { mail.composeModal = true; mail.compose = c } } })
    }
  })
}

// Called when a letter is opened: brings back the reply the user started on it.
async function restoreParked(m: FullMessage, seq: number) {
  const k = `${m.account}|${m.folder}|${m.uid}`
  const mem = parked.get(k)
  if (mem) {
    parked.delete(k)
    if (seq === openSeq && !mail.compose) { mail.composeModal = false; mail.composeView = 'open'; mail.compose = mem }
    return
  }
  const ref = parkedStore()[k]
  if (!ref || mail.compose) return
  const ok = await openDraft({ account: ref.account, folder: draftsFolder(ref.account), uid: ref.uid } as Summary, { inline: true, quiet: true })
  if (!ok) setParked(k, null) // the draft was sent or deleted elsewhere
}

export async function discardDraft() {
  const c = mail.compose
  if (!c) return
  forgetParked(c)
  mail.compose = null
  mail.composeModal = false
  const pending = saving.get(c)
  if (pending) await pending
  for (const u of c.uploads) u.xhr?.abort() // unfinished uploads stop
  for (const l of c.links) dropUpload(c, l.token)
  if (c.draftUid) {
    try { await api('/api/drafts/delete', { method: 'POST', body: { account: c.draftAccount, uid: c.draftUid } }) } catch {}
    draftsShown()
    refreshFoldersSoon(300)
  }
  toast(c.draftUid ? t('mail.draftTrashed') : t('mail.draftDeleted'))
}

function draftsFolder(account: string) {
  return mail.folders[account]?.find(f => f.role === 'drafts')?.name || 'Drafts'
}
export const isDraftRow = (m: Summary) => m.folder === draftsFolder(m.account)

async function openDraftRef(ref: { account: string, uid: number }) {
  return openDraft({ account: ref.account, folder: draftsFolder(ref.account), uid: ref.uid } as Summary)
}

// The viewer HTML of a draft, turned back into editor HTML: inline images keep their Content-ID,
// and the wrapper div added when the draft was saved is removed so it does not nest on every save.
function draftEditorHtml(html: string) {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('img').forEach(img => {
    const m = (img.getAttribute('src') || '').match(/[?&]cid=([^&]+)/)
    if (m) img.setAttribute('data-cid', decodeURIComponent(m[1]))
  })
  let body = doc.body
  const only = body.children.length === 1 ? body.firstElementChild as HTMLElement : null
  if (only?.tagName === 'DIV' && /font-family/.test(only.getAttribute('style') || '') && !only.hasAttribute('data-tmail-sig')) body = only as any
  return body.innerHTML
}

// openDraft loads a saved draft back into the compose window.
export async function openDraft(m: Summary, opts: { inline?: boolean, quiet?: boolean } = {}): Promise<boolean> {
  if (mail.compose) await closeCompose()
  let full: FullMessage & { bcc: Addr[], text?: string, replyFolder?: string, replyUid?: string }
  try {
    full = await api('/api/message', { query: { account: m.account, folder: m.folder, uid: m.uid, text: '1', peek: '1' } })
  } catch (e: any) { if (!opts.quiet) toast(e.message, { kind: 'err' }); return false }
  const c = emptyCompose(full.inReplyTo ? 'reply' : 'new')
  c.from = senderAccount(full.from.email) || senderAccount(m.account)
  c.sourceAccount = full.replyFolder ? m.account : ''
  c.to = full.to.map(fmtAddr).join(', ')
  c.cc = (full.cc || []).map(fmtAddr).join(', ')
  c.bcc = (full.bcc || []).map(fmtAddr).join(', ')
  c.showCc = !!(c.cc || c.bcc)
  c.subject = full.subject
  c.html = draftEditorHtml(full.html)
  c.inReplyTo = full.inReplyTo
  // The server appends In-Reply-To to References when saving; drop it so it is not doubled.
  c.references = full.references.replace(full.inReplyTo, '').trim()
  c.replyFolder = full.replyFolder || ''
  c.replyUid = Number(full.replyUid) || 0
  // The draft itself is the source of its attachments and pasted images.
  c.carry = { account: m.account, folder: m.folder, uid: m.uid, parts: full.attachments.filter(a => !a.embedded) }
  c.draftAccount = m.account
  c.draftUid = m.uid
  c.savedAt = full.date
  c.saveState = 'saved'
  if (c.html.includes('data-tmail-sig')) c.sig = signatureBlock(c.from)
  if (opts.inline && mail.compose) return true
  mail.composeModal = !opts.inline
  mail.composeView = 'open'
  mail.compose = c
  return true
}

// Like Gmail: the text promises a file ("attached", "please find the invoice") but nothing is attached.
// Detection patterns, not interface text: Russian phrases ("in the attachment", "I am sending",
// "invoice", "contract"…) and their English equivalents.
const ATTACH_RE = /[^.!?\n]{0,60}(во вложени|в приложени|прикрепл|прилага|прикладыва|высылаю|направляю|отправляю|пересылаю|посылаю)[^.!?\n]{0,60}|[^.!?\n]{0,60}\b(attached|attachment|enclosed)\b[^.!?\n]{0,40}/i
const ATTACH_NOUN = /(влож|приложен|прикреп|прилага|прикладыва|файл|счет|счёт|договор|документ|прайс|кп\b|коммерческ|акт|сверк|накладн|спецификац|фото|скан|карточк|реквизит|attach|enclos)/i
function mentionsAttachment(c: Compose): string {
  if (c.files.length || c.carry?.parts.length || Object.keys(c.inline).length || c.links.length || c.html.includes('data-tmail-files')) return ''
  const doc = new DOMParser().parseFromString(c.html, 'text/html')
  doc.querySelectorAll('[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd],blockquote').forEach(n => n.remove())
  const text = (doc.body.innerText || doc.body.textContent || '') + '\n' + c.subject
  const m = text.match(ATTACH_RE)
  return m && ATTACH_NOUN.test(m[0]) ? m[0].trim().replace(/\s+/g, ' ') : ''
}

export async function sendCompose() {
  const c = mail.compose
  if (!c || c.sending) return
  c.error = ''
  if (!c.from) { c.error = t('mail.noSendMailbox'); return }
  if (!(c.to + c.cc + c.bcc).trim()) { c.error = t('mail.noRecipient'); return }
  if (c.uploads.some(u => !u.error)) { c.error = t('mail.waitUploads'); return }
  if (!c.subject.trim() && !await confirmDialog(t('mail.noSubjectAsk'), '', { ok: t('common.send') })) return
  const said = mentionsAttachment(c)
  if (said && !await confirmDialog(t('mail.forgotAttachment'), t('mail.forgotAttachmentText', { said }), { ok: t('mail.sendWithoutFile') })) return
  const total = c.files.reduce((n, f) => n + f.size, 0) + Object.values(c.inline).reduce((n, f) => n + f.size, 0)
    + (c.carry?.parts.reduce((n, p) => n + p.size, 0) || 0)
  if (total > 25 * 1024 * 1024) { c.error = t('mail.tooBig'); return }
  const pending = saving.get(c)
  if (pending) await pending
  c.sending = true
  // Like Gmail: the window closes, the letter leaves in 15 s unless «Undo» brings it back.
  if (mail.compose === c) { mail.compose = null; mail.composeModal = false }
  if (await sendCountdown()) {
    c.sending = false
    await reopenCompose(c)
    return
  }
  try {
    const res = await api<{ account: string, folder: string, uid: number }>('/api/send', { method: 'POST', body: composeForm(c, c.files).fd })
    if (c.replyUid && (c.mode === 'reply' || c.mode === 'replyall')) {
      const same = (m: Summary) => m.folder === c.replyFolder && m.uid === c.replyUid && (!c.sourceAccount || m.account === c.sourceAccount)
      mail.messages.filter(same).forEach(m => { m.answered = true })
      if (mail.current && same(mail.current)) mail.current.answered = true
    }
    forgetParked(c)
    toast(t('mail.sentFrom', { from: c.from }), { click: () => openSent(res), action: { label: t('common.open'), run: () => openSent(res) }, ms: 7000 })
    if (c.mode === 'reply' || c.mode === 'replyall') setTimeout(refreshThread, 1500) // the copy in Sent is there by then
    if (mail.view.role === 'sent' || mail.view.role === 'drafts') loadMessages(false, true)
    refreshFoldersSoon(500)
  } catch (e: any) {
    c.error = e instanceof ApiError ? e.message : t('mail.sendFailed')
    c.sending = false
    toast(t('mail.notSent', { error: c.error }), { kind: 'err', ms: 8000 })
    await reopenCompose(c)
  } finally {
    c.sending = false
  }
}

const SEND_DELAY = 15
let sendsWaiting = 0
// sendCountdown shows «Sending in N s · Undo»; true when cancelled.
function sendCountdown(): Promise<boolean> {
  sendsWaiting++
  return new Promise(resolve => {
    let left = SEND_DELAY
    const label = () => t('mail.sendIn', { n: left })
    let timer: ReturnType<typeof setInterval>
    const finish = (cancelled: boolean) => { clearInterval(timer); dismissToast(note.id); sendsWaiting--; resolve(cancelled) }
    const note = toast(label(), { keep: true, ms: (SEND_DELAY + 5) * 1000, action: { label: t('mail.undo'), run: () => finish(true) } })
    timer = setInterval(() => {
      left--
      if (left <= 0) finish(false)
      else note.text = label()
    }, 1000)
  })
}

// reopenCompose brings a letter back into its window for editing (cancelled or not sent).
async function reopenCompose(c: Compose) {
  if (mail.compose && mail.compose !== c) await closeCompose()
  mail.compose = c
  mail.composeModal = true
  mail.composeView = 'open'
}

// openSent shows the letter just sent in the Sent folder.
function openSent(r: { account: string, folder: string, uid: number }) {
  setView({ role: 'sent' })
  if (r.folder && r.uid) openMessage({ account: r.account, folder: r.folder, uid: r.uid, seen: true } as Summary)
}

// Leaving the page while a letter waits its 15 s: the browser asks first (the draft stays saved).
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', e => { if (sendsWaiting > 0) { e.preventDefault(); e.returnValue = '' } })
}

// ---------- live updates ----------

// A new INBOX message arrived (pushed by the server). Returns true when it was added to the list.
// A new letter of the conversation that is open: offered at the top of it, like Gmail.
function joinsOpenThread(m: Summary) {
  const cur = mail.current
  if (!cur || cur.account !== m.account) return false
  const ids = new Set([cur, ...mail.thread].flatMap(x => [x.messageId, x.inReplyTo || '', ...(x.refs || [])]).filter(Boolean))
  if (m.inReplyTo && ids.has(m.inReplyTo)) return true
  if ((m.refs || []).some(r => ids.has(r))) return true
  // No references (some clients): the same subject from someone already in the conversation.
  const people = new Set([cur, ...mail.thread].flatMap(x => [x.from.email, ...x.to.map(a => a.email)]).map(e => e.toLowerCase()))
  return !!baseSubject(m.subject) && baseSubject(m.subject) === baseSubject(cur.subject) && people.has(m.from.email.toLowerCase())
}

export function receiveNew(m: Summary): boolean {
  threadCache.clear() // a conversation fetched ahead may have just grown
  if (keyOf(m) !== (mail.current ? keyOf(mail.current) : '') && joinsOpenThread(m)) mail.threadNew = m
  const inScope = !mail.view.folder && mail.view.role === 'inbox'
    && (!mail.view.account || mail.view.account === m.account)
    && !mail.filters.q && !mail.filters.since && !mail.filters.until
  refreshFoldersSoon(400)
  if (!inScope) return false
  // An old letter brought in by the mail collector would push every row down under the pointer
  // (clicks then hit another letter): it shows up in its place on the next reload.
  if (Date.now() - new Date(m.date).getTime() > 60 * 60 * 1000) return false
  if (mail.messages.some(x => keyOf(x) === keyOf(m))) return false
  m.fresh = true
  mail.messages.unshift(m)
  return true
}

export async function logout() {
  try { await api('/api/auth/logout', { method: 'POST' }) } catch {}
  mail.me = null
  await navigateTo('/login')
}

// ---------- large files: sent as links (storage on files.<domain>) ----------

export const ATTACH_LIMIT = 25 * 1024 * 1024
let uploadSeq = 0

export function attachedSize(c: Compose) {
  return c.files.reduce((n, f) => n + f.size, 0) + Object.values(c.inline).reduce((n, f) => n + f.size, 0)
    + (c.carry?.parts.reduce((n, p) => n + p.size, 0) || 0)
}

// Adds files to a letter: as attachments while they fit in 25 MB, otherwise as links.
// Programs are refused as attachments by mail servers (ours too): they always go as a link.
const EXEC_EXT = /\.(exe|scr|com|pif|bat|cmd|vbs|vbe|js|jse|wsf|wsh|hta|ps1|lnk|msi|msp|cpl|jar|reg|scf|chm|iso|img|vhdx?|xll|dll|appx|msix)$/i
export function addComposeFiles(c: Compose, files: File[]) {
  let small = false
  for (const f of files) {
    if (f.size > ATTACH_LIMIT || attachedSize(c) + f.size > ATTACH_LIMIT || EXEC_EXT.test(f.name)) uploadBig(c, f)
    else { c.files.push(f); small = true }
  }
  // To the server at once (into the draft), not a few seconds later or only when sending.
  if (small) saveDraft(c)
}

// Large files uploaded while writing this letter: removed from storage when the letter is
// dropped or the file taken out (links from earlier letters are left alone).
const ownUploads = new WeakMap<Compose, Set<string>>()
function dropUpload(c: Compose, token: string) {
  if (!ownUploads.get(c)?.has(token)) return
  ownUploads.get(c)!.delete(token)
  api(`/api/bigfiles/${encodeURIComponent(token)}`, { method: 'DELETE' }).catch(() => {})
}

function uploadBig(c: Compose, f: File) {
  const u: BigUpload = reactive({ id: ++uploadSeq, name: f.name, size: f.size, loaded: 0 })
  c.uploads.push(u)
  const xhr = new XMLHttpRequest()
  u.xhr = xhr
  const q = new URLSearchParams({ name: f.name, size: String(f.size), mailbox: c.from })
  xhr.open('POST', '/api/bigfiles?' + q)
  xhr.setRequestHeader('X-ZM', '1')
  if (f.type) xhr.setRequestHeader('Content-Type', f.type)
  xhr.upload.onprogress = e => { u.loaded = e.loaded }
  xhr.onload = () => {
    let res: any = null
    try { res = JSON.parse(xhr.responseText) } catch {}
    if (xhr.status === 200 && res?.token) {
      c.uploads = c.uploads.filter(x => x !== u)
      c.links.push(res as BigFile)
      if (!ownUploads.has(c)) ownUploads.set(c, new Set())
      ownUploads.get(c)!.add(res.token)
      renderFilesCard(c)
    } else {
      u.error = res?.error || t('mail.uploadFailed', { status: xhr.status })
    }
  }
  xhr.onerror = () => { u.error = t('mail.uploadOffline') }
  xhr.send(f)
}

export function cancelUpload(c: Compose, u: BigUpload) {
  u.xhr?.abort()
  c.uploads = c.uploads.filter(x => x !== u)
}

export function removeLink(c: Compose, token: string) {
  dropUpload(c, token)
  c.links = c.links.filter(l => l.token !== token)
  renderFilesCard(c)
}

const EXT_BG: Record<string, string> = { PDF: '#d93025', DOC: '#1a73e8', DOCX: '#1a73e8', XLS: '#188038', XLSX: '#188038', ZIP: '#5f6368', RAR: '#5f6368', '7Z': '#5f6368',
  MP4: '#a142f4', MOV: '#a142f4', AVI: '#a142f4', PPT: '#e8710a', PPTX: '#e8710a', JPG: '#a142f4', PNG: '#a142f4', DWG: '#12857e' }

// The card in the letter body: email-safe table with a small preview, name, size and a link.
function filesCardHtml(links: BigFile[]) {
  if (!links.length) return ''
  const exp = new Date(links.reduce((m, l) => l.expires < m ? l.expires : m, links[0].expires))
  // The card goes into the letter in the writer's interface language.
  const size = fileSize(links.reduce((n, l) => n + l.size, 0))
  const until = fmtDate(exp, { day: 'numeric', month: 'long' })
  const head = links.length > 1 ? t('mail.filesCard.headMany', { n: links.length, size, date: until }) : t('mail.filesCard.head', { size, date: until })
  const rows = links.map((l, i) => {
    const ext = (l.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase()
    const pic = l.thumb
      ? `<img src="${escHtml(l.thumb)}" width="72" height="54" alt="" style="display:block;width:72px;height:54px;border-radius:6px;border:1px solid #e0e3e7;object-fit:cover">`
      : `<div style="width:72px;height:54px;border-radius:6px;background:${EXT_BG[ext] || '#5f6368'};color:#ffffff;font:bold 13px/54px Arial,sans-serif;text-align:center">${escHtml(ext)}</div>`
    const border = i < links.length - 1 ? 'border-bottom:1px solid #eceff1;' : ''
    return `<tr><td style="padding:10px 14px;${border}"><a href="${escHtml(l.url)}" target="_blank" style="text-decoration:none;color:#202124;display:block">`
      + `<table cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tr><td width="72" valign="middle">${pic}</td>`
      + `<td valign="middle" style="padding-left:12px;font-family:Arial,Helvetica,sans-serif"><div style="font-size:14px;font-weight:bold;color:#202124;word-break:break-all">${escHtml(l.name)}</div>`
      + `<div style="font-size:12px;color:#5f6368;margin-top:2px">${escHtml(fileSize(l.size))} · <span style="color:#1a73e8;font-weight:bold">${escHtml(t('common.download'))}</span></div></td></tr></table></a></td></tr>`
  }).join('')
  return `<div data-tmail-files="" style="margin:14px 0"><table cellpadding="0" cellspacing="0" border="0" style="border-collapse:separate;border:1px solid #dadce0;border-radius:12px;width:100%;max-width:540px;font-family:Arial,Helvetica,sans-serif">`
    + `<tr><td style="padding:9px 14px;font-size:12px;color:#5f6368;background:#f8f9fa;border-bottom:1px solid #eceff1;border-radius:12px 12px 0 0">📎 ${escHtml(head)}</td></tr>${rows}</table></div>`
}

// Puts (or refreshes) the card after the user's text, before the signature and the quote.
function renderFilesCard(c: Compose) {
  const doc = new DOMParser().parseFromString(`<body>${c.html}</body>`, 'text/html')
  doc.querySelectorAll('[data-tmail-files]').forEach(n => n.remove())
  const html = filesCardHtml(c.links)
  if (html) {
    const tmp = doc.createElement('div')
    tmp.innerHTML = html
    const card = tmp.firstElementChild!
    const marker = Array.from(doc.body.children).find(n => n.matches('[data-tmail-sig],[data-tmail-quote],[data-tmail-fwd]'))
    if (marker) doc.body.insertBefore(card, marker)
    else doc.body.append(card)
  }
  c.html = doc.body.innerHTML
}

// Pictures from the internet in a letter (like Gmail): hidden until "Show", unless the user
// chose "always" or trusts this sender. Never automatically in spam.
export function imagesAllowed(m: { from: Addr, spam?: boolean }) {
  if (mail.showImages) return true
  if (m.spam || !mail.me) return false
  return mail.me.imagePolicy === 'always' || !!mail.me.imageSenders?.includes((m.from.email || '').toLowerCase())
}
export async function setImagePrefs(body: { policy?: 'ask' | 'always', allow?: string, remove?: string }) {
  const r = await api<{ imagePolicy: 'ask' | 'always', imageSenders: string[] }>('/api/me/images', { method: 'POST', body })
  if (mail.me) Object.assign(mail.me, r)
}
