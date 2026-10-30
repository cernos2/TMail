import { reactive } from 'vue'
import { mail, visibleThreads, type Attachment } from '~/composables/mail'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t, i18n } from '~/composables/i18n'

// Attachment links and the shared full-screen viewer. The viewer can be opened from the open letter
// or straight from the message list, and its arrows go through the files of all letters in the list.
export type MsgRef = { account: string, folder: string, uid: number }
export type FileRef = { msg: MsgRef, att: Attachment }

export function attUrl(m: MsgRef, a: Attachment, download = false, as = '') {
  const q = new URLSearchParams({ account: m.account, folder: m.folder, uid: String(m.uid), part: a.part })
  if (download) q.set('download', '1')
  if (as) q.set('as', as), q.set('pv', '2') // bump when the server-side conversion changes
  return `/api/attachment?${q}`
}

// ---------- antivirus warnings ----------
// What the antivirus found in the open letters (X-Virus), so every download link can warn.
const avOf = reactive<Record<string, string[]>>({})
const msgKey = (m: MsgRef) => `${m.account}|${m.folder}|${m.uid}`
export function setAV(m: MsgRef, av: string[] = []) { avOf[msgKey(m)] = av }

const ARCHIVE = /\.(zip|rar|7z|tar|gz|tgz|bz2|xz|arj|cab)$/i
const OFFICE = /\.(docx?|xlsx?|pptx?|docm|xlsm|pptm|dotm|xltm|rtf|odt|ods)$/i
const PDF = /\.pdf$/i
export type Risk = '' | 'virus' | 'exec' | 'macro' | 'encrypted' | 'unchecked' | 'html' | 'spam' | 'heuristic'
export function riskOf(m: MsgRef, a: Attachment): Risk {
  const av = avOf[msgKey(m)] || []
  if (av.includes('virus')) return 'virus'
  if (a.risk) return a.risk
  if (av.includes('macro') && OFFICE.test(a.name)) return 'macro'
  if (av.includes('encrypted') && (ARCHIVE.test(a.name) || OFFICE.test(a.name) || PDF.test(a.name))) return 'encrypted'
  if (av.includes('unchecked')) return 'unchecked'
  if (av.includes('heuristic')) return 'heuristic'
  if (av.includes('spam')) return 'spam'
  return ''
}
// [title, explanation] of each risk; getters, so the texts follow a language switch.
export const RISK_TEXT: Record<Exclude<Risk, ''>, [string, string]> = {
  get virus(): [string, string] { return [t('attach.riskVirus'), t('attach.riskVirusText')] },
  get exec(): [string, string] { return [t('attach.riskExec'), t('attach.riskExecText')] },
  get macro(): [string, string] { return [t('attach.riskMacro'), t('attach.riskMacroText')] },
  get encrypted(): [string, string] { return [t('attach.riskEncrypted'), t('attach.riskEncryptedText')] },
  get unchecked(): [string, string] { return [t('attach.riskUnchecked'), t('attach.riskUncheckedText')] },
  get html(): [string, string] { return [t('attach.riskHtml'), t('attach.riskHtmlText')] },
  get spam(): [string, string] { return [t('attach.riskSpam'), t('attach.riskSpamText')] },
  get heuristic(): [string, string] { return [t('attach.riskHeuristic'), t('attach.riskHeuristicText')] },
}
// ---------- antivirus verdicts: files are handed out only after a clean check ----------
export type Scan = { status: 'checking' | 'clean' | 'virus' | 'locked' | 'pending', virus?: string, kind?: string }
export const scans = reactive<Record<string, Scan>>({})
const fileKey = (m: MsgRef, a: Attachment) => `${msgKey(m)}|${a.part}`
export const scanOf = (m: MsgRef, a: Attachment): Scan | undefined => scans[fileKey(m, a)]

// scanAttachment asks the server for the verdict (it scans on first request, then remembers).
// With wait=true a busy antivirus is asked again for up to two minutes.
export async function scanAttachment(m: MsgRef, a: Attachment, wait = false): Promise<Scan> {
  const k = fileKey(m, a)
  const known = scans[k]
  if (known && (known.status === 'clean' || known.status === 'virus' || known.status === 'locked')) return known
  const deadline = Date.now() + 120_000
  for (;;) {
    if (!scans[k] || scans[k].status !== 'checking') scans[k] = { status: 'checking' }
    let v: Scan = { status: 'pending' }
    try {
      const r = await fetch(attUrl(m, a, false, 'scan'), { credentials: 'same-origin', headers: { 'X-ZM': '1' } })
      if (r.ok) { const j = await r.json(); v = { status: j.status, virus: j.virus, kind: j.kind } }
    } catch {}
    if (v.status !== 'pending' || !wait || Date.now() > deadline) { scans[k] = v; return v }
    await new Promise(res => setTimeout(res, 5000))
  }
}

// guardDownload: the file is downloaded only when the antivirus found it clean, and a risky
// kind of file (program, macros, password-protected) asks once more.
// Why a file that cannot be checked is not handed out.
export const lockedText = (a: Attachment, v: Scan) => v.kind === 'encrypted'
  ? t('attach.lockedEncrypted', { name: a.name })
  : v.kind === 'heuristic'
    ? t('attach.lockedHeuristic', { name: a.name, virus: v.virus })
    : t('attach.lockedTooBig', { name: a.name })

export async function guardDownload(e: Event, m: MsgRef, a: Attachment, url = attUrl(m, a, true)) {
  e.preventDefault()
  let v = scanOf(m, a)
  if (!v || v.status === 'checking' || v.status === 'pending') {
    toast(t('attach.scanningFile', { name: a.name }))
    v = await scanAttachment(m, a, true)
  }
  if (v.status === 'virus') { toast(t('attach.virusFound', { name: a.name, virus: v.virus }), { kind: 'err', ms: 9000 }); return }
  if (v.status === 'locked') { toast(lockedText(a, v), { kind: 'err', ms: 10000 }); return }
  if (v.status !== 'clean') { toast(t('attach.notScannedFile'), { kind: 'err', ms: 7000 }); return }
  const r = riskOf(m, a)
  if (r) {
    const [title, text] = RISK_TEXT[r]
    if (!await confirmDialog(t('attach.riskConfirm', { title, name: a.name }), text, { ok: t('attach.downloadAnyway'), danger: true })) return
  }
  location.href = url
}

export const ext = (a: Attachment) => (a.name.includes('.') ? a.name.split('.').pop()! : a.mime.split('/')[1] || '').slice(0, 4).toUpperCase()
export const EXT_COLORS: Record<string, string> = { PDF: '#BE3327', XLSX: '#1E8048', XLS: '#1E8048', XLSM: '#1E8048', DOCX: '#3D5FA8', DOC: '#3D5FA8', ZIP: '#6B7480', RAR: '#6B7480', '7Z': '#6B7480',
  JPG: '#8A4B9C', JPEG: '#8A4B9C', PNG: '#8A4B9C', GIF: '#8A4B9C', WEBP: '#8A4B9C', PPT: '#C4572B', PPTX: '#C4572B', ODT: '#3D5FA8', ODS: '#1E8048', CSV: '#1E8048', TXT: '#5B6573', RTF: '#3D5FA8' }
export const extColor = (a: Attachment) => EXT_COLORS[ext(a)] || '#6B7480'
export const canPreview = (a: Attachment) => !!a.preview || a.inline

export type Sheet = { name: string, doc: string }
export type OcrResult = { pages: { w: number, h: number, words: { t: string, x: number, y: number, w: number, h: number, l: number }[] }[], text: string }
type State = {
  items: FileRef[], index: number, loading: boolean,
  text?: string, url?: string, sheets?: Sheet[], sheet: number, error?: string, checking?: boolean,
  blocked?: boolean, // the antivirus did not let the file out: no download button next to the error
  ocr?: OcrResult | null, ocrBusy: boolean, scan: boolean, // scan: show PDF pages as pictures with the text layer
  zoom: number, // 0 = fit the window, otherwise a factor (1 = 100%)
}
export const pv = reactive<State>({ items: [], index: -1, loading: false, sheet: 0, ocrBusy: false, scan: false, zoom: 0 })
export const current = () => (pv.index >= 0 ? pv.items[pv.index] : null)
let seq = 0

const same = (a: FileRef, b: FileRef) => a.msg.account === b.msg.account && a.msg.folder === b.msg.folder && a.msg.uid === b.msg.uid && a.att.part === b.att.part

// Files of every letter in the list (newest conversation first), plus the ones passed in if the
// list doesn't have them (letters from Sent shown inside a conversation, for example).
function allFiles(extra: FileRef[]): FileRef[] {
  const out: FileRef[] = []
  for (const th of visibleThreads.value) {
    for (const m of th.items) for (const a of m.attachments || []) out.push({ msg: { account: m.account, folder: m.folder, uid: m.uid }, att: a })
  }
  if (extra.length && !out.some(x => same(x, extra[0]))) {
    const cur = mail.current
    const at = cur ? out.findIndex(x => x.msg.uid === cur.uid && x.msg.folder === cur.folder && x.msg.account === cur.account) : -1
    out.splice(at >= 0 ? at : 0, 0, ...extra)
  }
  return out
}

function reset() {
  seq++
  if (pv.url) URL.revokeObjectURL(pv.url)
  Object.assign(pv, { loading: false, text: undefined, url: undefined, sheets: undefined, sheet: 0, error: undefined, blocked: false, checking: false, ocr: undefined, ocrBusy: false, scan: false, zoom: 0 })
}

export function closePreview() {
  reset()
  pv.items = []
  pv.index = -1
}

// Opens the viewer on a file; `siblings` are the other files of the same letter.
export function openAttachment(msg: MsgRef, a: Attachment, siblings: Attachment[] = [a]) {
  const ref = { account: msg.account, folder: msg.folder, uid: msg.uid }
  const own = siblings.map(att => ({ msg: ref, att }))
  const items = allFiles(own)
  pv.items = items
  show(Math.max(0, items.findIndex(x => same(x, { msg: ref, att: a }))))
}

export function stepPreview(d: number) {
  if (pv.items.length < 2) return
  show((pv.index + d + pv.items.length) % pv.items.length)
}

async function show(i: number) {
  reset()
  pv.index = i
  const f = pv.items[i]
  if (!f) return
  const kind = f.att.preview
  {
    // Nothing of the file is shown before the antivirus has checked it.
    const my = ++seq
    pv.loading = true; pv.checking = true
    const v = await scanAttachment(f.msg, f.att, true)
    if (my !== seq) return
    pv.loading = false; pv.checking = false
    if (v.status === 'virus') { pv.blocked = true; pv.error = t('attach.virusFoundOpen', { virus: v.virus }); return }
    if (v.status === 'locked') { pv.blocked = true; pv.error = lockedText(f.att, v); return }
    if (v.status !== 'clean') { pv.blocked = true; pv.error = t('attach.notScannedOpen'); return }
  }
  if (kind === 'image') { runOcr(); return } // pictures: recognise text right away (cached on the server)
  if (kind !== 'text' && kind !== 'office' && kind !== 'sheet') return
  const my = ++seq
  pv.loading = true
  try {
    const r = await fetch(attUrl(f.msg, f.att, false, kind === 'text' ? 'text' : kind === 'sheet' ? 'html' : 'pdf'), { credentials: 'same-origin', headers: { 'X-ZM': '1' } })
    if (my !== seq) return
    if (!r.ok) {
      const j = await r.json().catch(() => null)
      throw new Error(j?.error || t('attach.serverError', { status: r.status }))
    }
    if (kind === 'text') { const txt = await r.text(); if (my === seq) pv.text = txt }
    else if (kind === 'sheet') { const h = await r.text(); if (my === seq) pv.sheets = splitSheets(h) }
    else { const b = await r.arrayBuffer(); if (my === seq) pv.url = URL.createObjectURL(new Blob([b], { type: 'application/pdf' })) }
  } catch (e: any) {
    if (my === seq) pv.error = e.message || t('attach.openFailed')
  } finally {
    if (my === seq) pv.loading = false
  }
}

// ---------- spreadsheet grid ----------

const colName = (n: number) => { let s = ''; for (n++; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + (n - 1) % 26) + s; return s }

const SHEET_CSS = `
html,body{margin:0;background:#fff;color:#000}
body{font:13px Arial,Helvetica,sans-serif}
table{border-collapse:collapse;table-layout:auto}
td{border:1px solid #e3e3e3;padding:2px 5px;vertical-align:bottom;white-space:pre-wrap}
th{background:#f1f3f4;color:#5f6368;font:12px Arial,sans-serif;border:1px solid #c7c7c7;padding:2px 4px;text-align:center;min-width:26px;user-select:none}
thead th{position:sticky;top:0;z-index:2}
th.rn{position:sticky;left:0;z-index:1;min-width:34px}
thead th.rn{z-index:3}
::selection{background:#c3dafe}`

// LibreOffice writes one table per sheet (with an <h1> name when there are several). Each sheet becomes
// a standalone document with Excel-like column letters and row numbers.
// LibreOffice's "Sheet N: " prefix before a sheet name, in English or Russian ("Лист" = "Sheet").
const SHEET_PREFIX = /^\s*(Sheet|Лист)\s*\d+\s*:\s*/i
function splitSheets(html: string): Sheet[] {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const out: Sheet[] = []
  doc.querySelectorAll('table').forEach((table, i) => {
    let name = ''
    for (let el = table.previousElementSibling; el; el = el.previousElementSibling) {
      if (el.tagName === 'TABLE') break
      const h = el.tagName === 'H1' ? el : el.querySelector('h1')
      if (h) { name = (h.textContent || '').replace(SHEET_PREFIX, '').trim(); break }
    }
    const rows = Array.from(table.rows)
    let cols = 0
    for (const r of rows) cols = Math.max(cols, Array.from(r.cells).reduce((n, c) => n + (c.colSpan || 1), 0))
    const head = doc.createElement('thead')
    const hr = doc.createElement('tr')
    hr.innerHTML = '<th class="rn"></th>' + Array.from({ length: cols }, (_, c) => `<th>${colName(c)}</th>`).join('')
    head.append(hr)
    rows.forEach((r, n) => { const th = doc.createElement('th'); th.className = 'rn'; th.textContent = String(n + 1); r.prepend(th) })
    table.prepend(head)
    table.querySelectorAll('colgroup').forEach(cg => cg.remove())
    out.push({ name: name || t('attach.sheetN', { n: i + 1 }), doc: `<!doctype html><html lang="${i18n.lang}"><head><meta charset="utf-8"><style>${SHEET_CSS}</style></head><body>${table.outerHTML}</body></html>` })
  })
  return out
}

// Text recognition (OCR) for the open picture, or for a scanned PDF (then shown page by page).
export async function runOcr(scan = false) {
  const f = current()
  if (!f) return
  const my = seq
  pv.ocrBusy = true
  if (scan) pv.scan = true
  try {
    const r = await fetch(attUrl(f.msg, f.att, false, 'ocr'), { credentials: 'same-origin', headers: { 'X-ZM': '1' } })
    if (!r.ok) throw new Error()
    const res = await r.json() as OcrResult
    if (my === seq) pv.ocr = res
  } catch {
    if (my === seq) pv.ocr = null
  } finally {
    if (my === seq) pv.ocrBusy = false
  }
}
export const pageUrl = (f: FileRef, n: number) => attUrl(f.msg, f.att, false, 'page') + '&n=' + n

// Zoom: steps like in document viewers; 0 = fit the window.
const STEPS = [0.25, 0.33, 0.5, 0.67, 0.75, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3, 4]
export function zoomBy(dir: 1 | -1, base = 1) {
  const z = pv.zoom || base
  const next = dir > 0 ? STEPS.find(s => s > z + 0.001) : [...STEPS].reverse().find(s => s < z - 0.001)
  pv.zoom = next ?? z
}
export const zoomFit = () => { pv.zoom = 0 }
export const zoomActual = () => { pv.zoom = 1 }
