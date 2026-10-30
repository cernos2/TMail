// The address bar follows what is on screen (mailbox, folder, label, open letter, contact card,
// search), so a link can be copied to another browser or sent to a colleague, and Back works.
//   /?box=info@firma.ru&f=sent&m=info@firma.ru/Sent/123
//   /?crm=42        /?label=3        /?folder=Suppliers&q=invoice
import { watch } from 'vue'
import type { Router, LocationQuery } from 'vue-router'
import { mail, setView, openMessage, closeMessage, loadMessages, type Summary } from './mail'
import { crm, openParty, loadCrmList } from './crm'

type Q = Record<string, string>

// What is on screen, as query parameters.
export function stateQuery(): Q {
  const q: Q = {}
  const v = mail.view
  if (v.account) q.box = v.account
  if (v.label != null) q.label = String(v.label)
  else if (v.folder) q.folder = v.folder
  else if (v.role && v.role !== 'inbox') q.f = v.role
  if (mail.filters.q) q.q = mail.filters.q
  if (mail.selected) {
    const [account, ...rest] = mail.selected.split('|')
    const uid = rest.pop()
    q.m = `${account}/${rest.join('|')}/${uid}`
  } else if (crm.open) {
    q.crm = crm.current ? String(crm.current.id) : ''
  }
  return q
}

const str = (x: unknown) => (Array.isArray(x) ? x[0] : x) as string | null | undefined

function sameQuery(a: Q, b: LocationQuery) {
  const kb = Object.keys(b).filter(k => b[k] != null)
  return Object.keys(a).length === kb.length && Object.keys(a).every(k => str(b[k]) === a[k])
}

// Puts the screen described by the query on: the folder list, then the letter or the card.
export async function applyQuery(q: LocationQuery, first = false) {
  const view = {
    account: str(q.box) || null,
    role: str(q.f) || 'inbox',
    folder: str(q.folder) || null,
    label: q.label ? Number(str(q.label)) : null,
  }
  const v = mail.view
  const search = str(q.q) || ''
  if (first || v.account !== view.account || v.role !== view.role || v.folder !== view.folder || v.label !== view.label || mail.filters.q !== search) {
    setView(view)
    if (search) { mail.filters.q = search; loadMessages() }
  }
  const m = str(q.m)
  if (m) {
    const i = m.indexOf('/'), j = m.lastIndexOf('/')
    const uid = Number(m.slice(j + 1))
    const key = `${m.slice(0, i)}|${m.slice(i + 1, j)}|${uid}`
    if (i > 0 && j > i && uid > 0 && mail.selected !== key) {
      openMessage({ account: m.slice(0, i), folder: m.slice(i + 1, j), uid, seen: true } as Summary)
    }
    return
  }
  if (mail.selected) closeMessage()
  if (q.crm != null) {
    const id = Number(str(q.crm))
    crm.open = true
    mail.reading = true
    if (!crm.list.length) loadCrmList()
    if (id > 0 && crm.current?.id !== id) openParty(id)
    else if (!id) crm.current = null
  } else if (crm.open) {
    crm.open = false
  }
}

let applying = false

// Keeps the address bar and the screen in step. Folder and letter changes are new history
// entries (Back returns); typing in search only replaces the current one.
export function syncUrl(router: Router) {
  watch(() => router.currentRoute.value.query, async q => {
    if (router.currentRoute.value.path !== '/' || sameQuery(stateQuery(), q)) return
    applying = true
    try { await applyQuery(q) } finally { applying = false }
  })
  watch(stateQuery, (next, prev) => {
    if (applying || router.currentRoute.value.path !== '/' || sameQuery(next, router.currentRoute.value.query)) return
    const onlySearch = prev && Object.keys({ ...next, ...prev }).every(k => k === 'q' || next[k] === prev[k])
    ;(onlySearch ? router.replace : router.push)({ path: '/', query: next })
  }, { deep: true })
}
