#!/usr/bin/env node
// TMAIL demo server: serves the built webmail (.output/public) and fakes the whole API with an
// invented company's mail, calendar, contacts and administration — for screenshots and videos.
// Nothing here touches a real mail server; every name and domain is fictional (.example).
//
//   cd web && npx nuxt generate && node demo/server.mjs --port 8790 --lang en   (or --lang ru)
//
// Options: --port N, --lang en|ru, --host 127.0.0.1, --auto-login (no sign-in page),
//          --remote-origin http://host:port (where "remote" newsletter pictures load from).
// Demo controls (no session needed):
//   POST /demo/push?n=1            the next invented letter arrives (event "new", or "filed")
//   POST /demo/push?id=n2          a given one (n1 new mail, n2 joins the open conversation,
//                                  n3 sales@, n4 auto-filed into Invoices)
//   POST /demo/reminder            the calendar reminder pops up again (/demo/reminder-done: never)
//   POST /demo/reset               back to the initial data
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import { fileURLToPath } from 'node:url'
import { world } from './world.mjs'
import { letters, incoming } from './letters.mjs'
import { extras } from './extras.mjs'
import { bodies } from './bodies.mjs'
import { aiAnswer, autoReplyText } from './ai.mjs'
import { zip } from './tools/zip.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const PUBLIC = path.join(HERE, '..', '.output', 'public')
const ASSETS = path.join(HERE, 'assets')

// ---------- options ----------
const argv = process.argv.slice(2)
const opt = (name, def) => { const i = argv.indexOf('--' + name); return i >= 0 ? (argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[i + 1] : true) : def }
const PORT = Number(opt('port', 8790))
const LANG = opt('lang', 'en') === 'ru' ? 'ru' : 'en'
const BIND = opt('host', '127.0.0.1')
const AUTO_LOGIN = !!opt('auto-login', false)
const REMOTE_ORIGIN = opt('remote-origin', '')
if (!fs.existsSync(path.join(PUBLIC, 'index.html'))) {
  console.error(`No built webmail in ${PUBLIC}: run "npx nuxt generate" in web/ first.`)
  process.exit(1)
}

// ---------- small helpers ----------
const ROLE_FOLDER = { inbox: 'INBOX', drafts: 'Drafts', sent: 'Sent', archive: 'Archive', junk: 'Junk', trash: 'Trash' }
const FOLDER_ROLE = Object.fromEntries(Object.entries(ROLE_FOLDER).map(([r, f]) => [f, r]))
const ROLE_ORDER = { inbox: 0, drafts: 1, sent: 2, archive: 3, junk: 4, trash: 5, '': 6 }
const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const sleep = ms => new Promise(r => setTimeout(r, ms))
const iso = d => new Date(d).toISOString()
const lower = s => String(s || '').toLowerCase()
const asset = rel => fs.readFileSync(path.join(ASSETS, rel))
const PUBLIC_DOMAINS = new Set(['gmail.example', 'mailbox.example'])

const MIME = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.mjs': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8',
  '.json': 'application/json', '.svg': 'image/svg+xml', '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.gif': 'image/gif', '.webp': 'image/webp',
  '.ico': 'image/x-icon', '.woff': 'font/woff', '.woff2': 'font/woff2', '.ttf': 'font/ttf', '.txt': 'text/plain; charset=utf-8', '.wasm': 'application/wasm',
  '.pdf': 'application/pdf', '.map': 'application/json', '.webmanifest': 'application/manifest+json', '.mp3': 'audio/mpeg', '.ogg': 'audio/ogg',
}

// Sample files the letters carry (built by tools/make-assets.mjs).
const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
const DOCX = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
function fileTable(lang) {
  const L = (en, ru) => (lang === 'ru' ? ru : en)
  const pdf = k => ({ mime: 'application/pdf', path: `${lang}/${k}.pdf`, thumb: `${lang}/${k}.thumb.png` })
  const csv = L(`time;bearing;velocity_mm_s;temperature_c
08:00;NDE;3.1;41
08:00;DE;9.8;47
10:00;NDE;3.4;42
10:00;DE;10.6;49
12:00;NDE;3.3;43
12:00;DE;11.2;51
14:00;NDE;3.5;43
14:00;DE;10.9;52
`, `время;подшипник;виброскорость_мм_с;температура_C
08:00;свободный конец;3,1;41
08:00;сторона привода;9,8;47
10:00;свободный конец;3,4;42
10:00;сторона привода;10,6;49
12:00;свободный конец;3,3;43
12:00;сторона привода;11,2;51
14:00;свободный конец;3,5;43
14:00;сторона привода;10,9;52
`)
  return {
    spec: pdf('spec'), offer: pdf('offer'), invoice: pdf('invoice'), po: pdf('po'),
    scan: { ...pdf('scan'), ocr: `${lang}/scan.ocr.json`, page: `${lang}/scan.page1.jpg` },
    contract: { mime: DOCX, path: `${lang}/contract.docx`, pdf: `${lang}/contract.pdf`, thumb: `${lang}/contract.thumb.png` },
    manual: { mime: DOCX, path: `${lang}/manual.docx`, pdf: `${lang}/manual.pdf`, thumb: `${lang}/manual.thumb.png` },
    pricelist: { mime: XLSX, path: `${lang}/pricelist.xlsx`, html: `${lang}/pricelist.html`, pdf: `${lang}/pricelist.pdf`, thumb: `${lang}/pricelist.thumb.png` },
    casting1: { mime: 'image/jpeg', path: 'common/casting1.jpg', ocrEmpty: [1200, 900] },
    casting2: { mime: 'image/jpeg', path: 'common/casting2.jpg', ocrEmpty: [1200, 900] },
    nameplate: { mime: 'image/jpeg', path: `${lang}/nameplate.jpg`, ocr: `${lang}/nameplate.ocr.json` },
    csv: { mime: 'text/csv', data: Buffer.from(csv) },
    macro: { mime: 'application/vnd.ms-word.document.macroEnabled.12', data: Buffer.from('PK\u0003\u0004 demo: not a real document, the antivirus blocks it'), virus: 'Doc.Dropper.Agent-7426316-0' },
  }
}
const fileBytes = f => f.data || asset(f.path)

// What the server's attachment code says about a file name / type.
const inlineSafe = mt => ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'application/pdf'].includes(mt)
const TEXT_EXT = new Set(['.txt', '.csv', '.log', '.md', '.json', '.xml', '.yaml', '.yml', '.ini', '.conf', '.cfg', '.tsv'])
const SHEET_EXT = new Set(['.xls', '.xlsx', '.xlsm', '.ods'])
const OFFICE_EXT = new Set(['.doc', '.docx', '.docm', '.rtf', '.odt', '.xls', '.xlsx', '.xlsm', '.ods', '.ppt', '.pptx', '.odp', '.pps', '.ppsx'])
function previewKind(mt, name) {
  const ext = path.extname(lower(name))
  if (mt.startsWith('image/') && inlineSafe(mt)) return 'image'
  if (mt === 'application/pdf') return 'pdf'
  if ((mt.startsWith('text/') && mt !== 'text/html') || mt === 'application/json' || mt === 'application/xml' || TEXT_EXT.has(ext)) return 'text'
  if (SHEET_EXT.has(ext)) return 'sheet'
  if (OFFICE_EXT.has(ext)) return 'office'
  return ''
}
const RISK = {}
for (const e of '.exe .scr .com .pif .bat .cmd .vbs .vbe .js .jse .wsf .wsh .wsc .hta .ps1 .psm1 .lnk .msi .msp .msc .cpl .jar .reg .scf .inf .chm .iso .img .vhd .vhdx .xll .sct .dll .sys'.split(' ')) RISK[e] = 'exec'
for (const e of '.docm .dotm .xlsm .xltm .xlam .pptm .potm .ppam .ppsm .sldm'.split(' ')) RISK[e] = 'macro'
for (const e of '.html .htm .shtml .svg .url'.split(' ')) RISK[e] = 'html'
const riskOf = name => RISK[path.extname(lower(name))] || ''
const mimeByName = name => ({ '.pdf': 'application/pdf', '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.gif': 'image/gif', '.webp': 'image/webp', '.txt': 'text/plain', '.csv': 'text/csv',
  '.docx': DOCX, '.xlsx': XLSX, '.zip': 'application/zip', '.doc': 'application/msword', '.xls': 'application/vnd.ms-excel' }[path.extname(lower(name))] || 'application/octet-stream')

// ---------- the demo state ----------
let S
function build() {
  const now = new Date()
  const W = world(LANG)
  const X = extras(W, now)
  const st = {
    now, W, X, B: bodies(W), files: fileTable(LANG), msgs: [], byId: {}, uid: {}, seq: 0,
    settings: {}, me: { lang: LANG, timezone: '', imagePolicy: 'ask', imageSenders: [], background: '', backgroundVersion: 0 },
    photos: {}, bgImage: null, sigImages: {}, bigfiles: {}, spellDict: new Set(), events: X.events, plan: { [monthKey(now)]: X.plan }, reminderDone: new Set(), snoozed: {},
    queue: incoming(W, now), pushed: 0, crm: null, avTested: 0,
  }
  S = st
  const specs = letters(W, now).sort((a, b) => a.at - b.at)
  for (const sp of specs) addLetter(sp)
  // The call about Tidewater's questions points at their letter.
  const tw5 = st.byId.tw5
  if (tw5 && st.events[0]) st.events[0].mailRef = `${tw5.account}|${tw5.folder}|${tw5.uid}`
  st.crm = buildCrm()
  return st
}
const monthKey = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`

// The mailbox Anna (the signed-in administrator) works with, as /api/me lists them.
function accounts() {
  const { W } = S
  const mine = W.mailboxes.filter(m => m.users.some(u => u[0] === 1) && m.active !== false && !m.deletedAt)
  const order = a => { const i = W.annaAccounts.indexOf(a.address); return i < 0 ? 100 + a.id : i }
  return mine.map(m => {
    const g = m.users.find(u => u[0] === 1)
    return { address: m.address, name: m.name, canSend: !!(g[1] || g[2]), personal: !!g[2], signature: W.signatures[m.address] ?? '', template: W.startTemplates[m.address] ?? '', canEditSignature: true }
  }).sort((a, b) => (b.personal - a.personal) || order(a) - order(b))
}
const accountOf = addr => accounts().find(a => a.address === lower(addr))
const mailboxByAddr = addr => S.W.mailboxes.find(m => m.address === lower(addr))

function folderNames(acct) { return [...Object.values(ROLE_FOLDER), ...(S.X.ownFolders[acct] || [])] }
function resolveFolder(acct, q) {
  if (q.folder) return q.folder
  return ROLE_FOLDER[q.role || 'inbox'] || null
}
function nextUid(acct, folder) {
  const k = acct + '\u0000' + folder
  S.uid[k] = (S.uid[k] || 100) + 1
  return S.uid[k]
}

// A person (key of the world's people), one of our mailboxes, or a plain address → {name, email}.
function who(x) {
  if (!x) return { name: '', email: '' }
  if (typeof x === 'object') return { name: x.name || '', email: lower(x.email) }
  const { W } = S
  if (W.P[x]) return { name: W.P[x].name, email: lower(W.P[x].email) }
  const mb = mailboxByAddr(x)
  if (mb) {
    const owner = mb.users.find(u => u[2])
    const u = owner && W.users.find(z => z.id === owner[0])
    return { name: mb.external ? W.U.anna.name : u ? u.name : mb.name, email: mb.address }
  }
  return { name: '', email: lower(x) }
}

function textHtml(t) {
  return String(t || '').trim().split(/\n{2,}/).map(p => `<p style="margin:0 0 12px">${esc(p).replace(/\n/g, '<br>')}</p>`).join('\n')
}
const htmlText = h => String(h || '').replace(/<(style|script)[\s\S]*?<\/\1>/gi, '').replace(/<br\s*\/?>/gi, '\n').replace(/<\/(p|div|tr|li|h\d)>/gi, '\n').replace(/<[^>]+>/g, '')
  .replace(/&nbsp;/g, ' ').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&amp;/g, '&').replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim()
const previewOf = t => String(t || '').replace(/\s+/g, ' ').trim().slice(0, 220)
function fmtLong(d) {
  return new Intl.DateTimeFormat(LANG === 'ru' ? 'ru-RU' : 'en-GB', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }).format(d)
}
function quoteHead(m) {
  const from = m.from.name ? `${m.from.name} <${m.from.email}>` : m.from.email
  return LANG === 'ru' ? `${fmtLong(m.date)}, ${from}:` : `On ${fmtLong(m.date)}, ${from} wrote:`
}
function signatureFor(acct, by) {
  const raw = S.W.signatures[acct]
  if (!raw) return ''
  const u = by && S.W.U[by]
  const sig = raw.replace(/\{name\}/g, u ? u.name : S.W.U.anna.name).replace(/\{email\}/g, acct).replace(/\{login\}/g, u ? u.login : 'anna')
  const inner = /<(div|p|br|a|img)\b/i.test(sig) ? sig : esc(sig).replace(/\n/g, '<br>')
  return `<div><br></div><div data-tmail-sig=""><div>--&nbsp;</div>${inner}</div>`
}
// The card "files by link" written into a letter (as the composer does).
function filesCard(links) {
  const ru = LANG === 'ru'
  const total = links.reduce((n, l) => n + l.size, 0)
  const exp = new Date(S.now.getTime() + 27 * 86400000)
  const date = new Intl.DateTimeFormat(ru ? 'ru-RU' : 'en-GB', { day: 'numeric', month: 'long' }).format(exp)
  const size = humanSize(total, ru)
  const head = ru ? `Файлы по ссылке · ${links.length} файла, ${size} · хранятся до ${date}` : `Files by link · ${links.length} files, ${size} · available until ${date}`
  const COL = { PDF: '#d93025', STEP: '#12857e' }
  const rows = links.map((l, i) => {
    const ext = (l.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase()
    const pic = l.thumb ? `<img src="{{ORIGIN}}/t/${l.token}.jpg" width="72" height="54" alt="" style="display:block;width:72px;height:54px;border-radius:6px;border:1px solid #e0e3e7;object-fit:cover">`
      : `<div style="width:72px;height:54px;border-radius:6px;background:${COL[ext] || '#5f6368'};color:#ffffff;font:bold 13px/54px Arial,sans-serif;text-align:center">${esc(ext)}</div>`
    const border = i < links.length - 1 ? 'border-bottom:1px solid #eceff1;' : ''
    return `<tr><td style="padding:10px 14px;${border}"><a href="{{ORIGIN}}/f/${l.token}" target="_blank" style="text-decoration:none;color:#202124;display:block"><table cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse"><tr><td width="72" valign="middle">${pic}</td><td valign="middle" style="padding-left:12px;font-family:Arial,Helvetica,sans-serif"><div style="font-size:14px;font-weight:bold;color:#202124;word-break:break-all">${esc(l.name)}</div><div style="font-size:12px;color:#5f6368;margin-top:2px">${esc(humanSize(l.size, ru))} · <span style="color:#1a73e8;font-weight:bold">${ru ? 'Скачать' : 'Download'}</span></div></td></tr></table></a></td></tr>`
  }).join('')
  return `<div data-tmail-files="" style="margin:14px 0"><table cellpadding="0" cellspacing="0" border="0" style="border-collapse:separate;border:1px solid #dadce0;border-radius:12px;width:100%;max-width:540px;font-family:Arial,Helvetica,sans-serif"><tr><td style="padding:9px 14px;font-size:12px;color:#5f6368;background:#f8f9fa;border-bottom:1px solid #eceff1;border-radius:12px 12px 0 0">📎 ${esc(head)}</td></tr>${rows}</table></div>`
}
function humanSize(n, ru) {
  const u = ru ? ['Б', 'КБ', 'МБ', 'ГБ'] : ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (n >= 1024 && i < 3) { n /= 1024; i++ }
  return (i ? n.toLocaleString(ru ? 'ru-RU' : 'en-GB', { maximumFractionDigits: 1 }) : n) + ' ' + u[i]
}

// A letter of the dataset (or one written in the demo) → the stored message.
function addLetter(sp) {
  const { W, B, files } = S
  const folder = ROLE_FOLDER[sp.folder] || sp.folder
  const from = who(sp.from)
  const ours = !!mailboxByAddr(from.email)
  const re = sp.re ? S.byId[sp.re] : null
  let html = sp.html ? (B[sp.html] ?? sp.html) : textHtml(sp.text)
  if (sp.links) {
    for (const l of sp.links) S.bigfiles[l.token] = { ...l, sender: from.email, expires: iso(new Date(S.now.getTime() + 27 * 86400000)) }
    html += filesCard(sp.links)
  }
  if (ours && sp.text) html += signatureFor(from.email, sp.by)
  if (re && !sp.noQuote) html += `<div data-tmail-quote=""><br><div>${esc(quoteHead(re))}</div><blockquote>${re.html}</blockquote></div>`
  const atts = (sp.atts || []).map((a, i) => {
    const f = files[a.file]
    const size = fileBytes(f).length
    return { part: String(i + 2), name: a.name, mime: f.mime, size, inline: inlineSafe(f.mime), preview: previewKind(f.mime, a.name), risk: riskOf(a.name), file: a.file }
  })
  const domain = from.email.split('@')[1] || 'nordwave.example'
  const messageId = `<${sp.id}.${crypto.createHash('md5').update(sp.id + LANG).digest('hex').slice(0, 10)}@${domain}>`
  const ext = !ours && !W.P[sp.from]?.own
  const auth = sp.auth || (ext && sp.folder !== 'junk' ? { spf: 'pass', dkim: 'pass', dmarc: sp.dmarc || 'none' } : {})
  const m = {
    id: sp.id, account: sp.acct, folder, uid: nextUid(sp.acct, folder), from, to: (sp.to || []).map(who), cc: (sp.cc || []).map(who), bcc: (sp.bcc || []).map(who),
    subject: sp.subject, date: sp.at, seen: !!sp.seen, flagged: !!sp.flagged, answered: !!sp.answered, html, text: sp.text ? sp.text : htmlText(html),
    preview: previewOf(sp.text || htmlText(sp.html ? (B[sp.html] ?? sp.html) : '')), atts, messageId,
    inReplyTo: re ? re.messageId : '', refs: re ? [...re.refs, re.messageId] : [], root: re ? re.root : messageId,
    labels: sp.labels || [], sentBy: sp.by && !mailboxByAddr(sp.acct)?.users.find(u => u[2]) ? { login: W.U[sp.by].login, name: W.U[sp.by].name } : null,
    dmarc: sp.dmarc || (auth.dmarc === 'pass' ? 'pass' : ''), auth, spam: !!sp.spam, av: sp.av || [], remote: !!sp.remote,
    replyFolder: sp.replyFolder || '', replyUid: sp.replyUid || '', cids: sp.cids || {}, extraFiles: sp.extraFiles || {},
  }
  m.size = Math.round(m.html.length * 1.1 + atts.reduce((n, a) => n + a.size * 1.37, 0)) + 2400
  S.msgs.push(m)
  S.byId[sp.id] = m
  return m
}

const keyOf = m => `${m.account}|${m.folder}|${m.uid}`
const find = (acct, folder, uid) => S.msgs.find(m => m.account === lower(acct) && m.folder === folder && m.uid === Number(uid))

function summary(m) {
  const out = {
    account: m.account, folder: m.folder, uid: m.uid, from: m.from, to: m.to,
    subject: m.subject, date: iso(m.date), seen: m.seen, flagged: m.flagged, answered: m.answered, hasAttachments: m.atts.length > 0,
    size: m.size, preview: m.preview, messageId: m.messageId,
  }
  if (m.cc.length) out.cc = m.cc
  if (m.atts.length) out.attachments = m.atts.map(attOut)
  if (m.inReplyTo) out.inReplyTo = m.inReplyTo
  if (m.refs.length) out.refs = m.refs
  if (m.labels.length) out.labels = m.labels
  if (m.sentBy) out.sentBy = m.sentBy
  if (m.dmarc) out.dmarc = m.dmarc
  return out
}
const attOut = a => { const o = { part: a.part, name: a.name, mime: a.mime, size: a.size, inline: a.inline, preview: a.preview }; if (a.risk) o.risk = a.risk; return o }
function fullMsg(m, req, withText) {
  const origin = originOf(req)
  let html = m.html.replace(/\{\{ORIGIN\}\}/g, origin).replace(/\{\{REMOTE\}\}/g, remoteOrigin(req))
  html = html.replace(/src="cid:([^"]+)"/g, (x, cid) => `src="/api/cid?${new URLSearchParams({ account: m.account, folder: m.folder, uid: String(m.uid), cid })}"`)
  const out = { ...summary(m), html, remoteImages: m.remote, attachments: m.atts.map(attOut), replyTo: [], inReplyTo: m.inReplyTo, references: m.refs.join(' '),
    spam: m.spam || m.folder === 'Junk', bcc: m.bcc, auth: m.auth, av: m.av }
  if (withText || m.folder === 'Drafts') out.text = m.text
  if (m.replyFolder) { out.replyFolder = m.replyFolder; out.replyUid = String(m.replyUid) }
  return out
}
const originOf = req => `http://${req.headers.host || `localhost:${PORT}`}`
function remoteOrigin(req) {
  if (REMOTE_ORIGIN) return REMOTE_ORIGIN
  const host = req.headers.host || `localhost:${PORT}`
  const [h, p] = host.split(':')
  if (h !== 'localhost' && h !== '127.0.0.1') return `http://127.0.0.1:${PORT}` // a mapped name (mail.nordwave.example)
  return `http://${h === 'localhost' ? '127.0.0.1' : 'localhost'}${p ? ':' + p : ''}`
}

// ---------- counterparty cards ----------
function buildCrm() {
  const { W } = S
  const parties = []
  let id = 0
  const keyId = {}
  const company = (k, c) => {
    const p = { id: ++id, kind: 'company', name: c.name, country: c.country || '', ids: c.ids || {}, inn: c.inn || '', kpp: c.kpp || '', ogrn: c.ogrn || '', legalAddress: c.legalAddress || '', bankName: c.bankName || '',
      bik: c.bik || '', account: c.account || '', corrAccount: c.corrAccount || '', website: c.website || '', position: '', notes: c.notes || '', phones: c.phones || [], emails: c.emails || [],
      domains: c.domains || [], own: !!c.own, companyId: null, companyName: '', addresses: [], suggestions: [], links: [] }
    parties.push(p); keyId[k] = p.id
    return p
  }
  for (const [k, c] of Object.entries(W.C)) company(k, c)
  const person = (k, p, coKey, own = false) => {
    const co = coKey ? parties.find(x => x.id === keyId[coKey]) : null
    const x = { id: ++id, kind: 'person', name: p.name, country: '', ids: {}, inn: '', kpp: '', ogrn: '', legalAddress: '', bankName: '', bik: '', account: '', corrAccount: '', website: '',
      position: p.position || '', notes: '', phones: p.phone ? [p.phone] : [], emails: [lower(p.email)], domains: [], own, companyId: co ? co.id : null, companyName: co ? co.name : '', addresses: [], suggestions: [] }
    parties.push(x); keyId['p:' + k] = x.id
    return x
  }
  for (const k of W.contacts) person(k, W.P[k], W.P[k].co)
  person('clara', W.P.clara, null)
  for (const k of ['anna', 'mark', 'sofia', 'peter', 'lena']) person(k, { ...W.P[k], phone: W.org.phone }, 'own', true)
  const P = k => parties.find(x => x.id === keyId[k])
  const L = W.L
  P('tide').addresses = [{ id: 1, kind: 'delivery', address: L('Tidewater yard, gate 3, Havenstraat 118, 3011 Rotterdam', 'Канонерский о-в, д. 5, КПП № 3, г. Санкт-Петербург') }]
  P('maple').addresses = [{ id: 2, kind: 'delivery', address: L('Pump Station 3, 118 Lakeshore Dr, Maple Ridge, BC', 'Насосная станция № 3, г. Приозерск, ул. Береговая, д. 2') }, { id: 3, kind: 'delivery', address: L('Central warehouse, 40 Industrial Way, Maple Ridge, BC', 'Центральный склад, г. Приозерск, ул. Заводская, д. 9') }]
  P('vistula').suggestions = [
    { id: 1, field: 'legal_address', value: L('ul. Przemysłowa 14, 03-218 Warszawa, Poland', 'Беларусь, 220075, г. Минск, ул. Промышленная, д. 14, пом. 3'), source: L('Partnership proposal: cast steel valve bodies', 'Предложение о сотрудничестве: стальные литые корпуса') },
    { id: 2, field: 'notes', value: L('Foundry, cast steel and duplex valve bodies DN50–DN600. Wants to become our supplier.', 'Литейное производство: корпуса задвижек DN50–DN600 из стали и дуплекса. Хочет стать нашим поставщиком.'), source: L('Partnership proposal: cast steel valve bodies', 'Предложение о сотрудничестве: стальные литые корпуса') },
  ]
  P('p:ruben').suggestions = [{ id: 3, field: 'position', value: L('Maintenance Lead, pump rooms 1–4', 'Главный механик, насосные 1–4'), source: L('URGENT: KP-200 vibrates after restart', 'СРОЧНО: КП-200 вибрирует после перезапуска') }]
  // Supply links between the companies.
  const own = P('own')
  const rel = (a, b, role) => ({ a: keyId[a], b: keyId[b], role }) // a supplies b
  S.crmLinks = [rel('bright', 'own'), rel('kestrel', 'own'), rel('alpen', 'own'), rel('hengda', 'own'), rel('sevarm', 'own'), rel('own', 'tide'), rel('own', 'maple'), rel('harbour', 'own', 'via')]
  S.crmNext = { party: id, addr: 10, sugg: 10 }
  void own
  return parties
}
// Letters that mention a card (by its people's addresses or its domains), in the user's mailboxes.
function partyLetters(p) {
  const emails = new Set()
  if (p.kind === 'company') {
    for (const x of S.crm) if (x.kind === 'person' && x.companyId === p.id) x.emails.forEach(e => emails.add(e))
  } else p.emails.forEach(e => emails.add(e))
  const doms = new Set(p.kind === 'company' && !p.own ? p.domains : [])
  const accs = new Set(accounts().map(a => a.address))
  return S.msgs.filter(m => accs.has(m.account) && m.folder !== 'Junk' && m.folder !== 'Trash' && m.folder !== 'Drafts' && [m.from, ...m.to, ...m.cc].some(a => emails.has(a.email) || doms.has(a.email.split('@')[1])))
}
function partyOut(p, full) {
  const ls = partyLetters(p)
  const last = p.own ? null : ls.reduce((d, m) => (!d || m.date > d ? m.date : d), null)
  const out = { ...p, letters: ls.length, pending: p.suggestions.length }
  if (last) out.lastLetter = iso(last)
  if (out.companyId == null) delete out.companyId
  if (!out.companyName) delete out.companyName
  delete out.addresses; delete out.suggestions; delete out.links
  if (!full) return out
  out.addresses = p.addresses
  if (p.suggestions.length) out.suggestions = p.suggestions.map(g => g.field === 'same' ? { ...g, target: S.crm.find(x => x.id === Number(g.value))?.name || '' } : g)
  const relOf = (x, role) => { const xs = partyLetters(x); const l = xs.reduce((d, m) => (!d || m.date > d ? m.date : d), null); return { id: x.id, name: x.name, own: !!x.own, role, letters: xs.length, ...(l ? { lastLetter: iso(l) } : {}) } }
  if (p.kind === 'company') {
    out.contacts = S.crm.filter(x => x.kind === 'person' && x.companyId === p.id).map(x => partyOut(x, false))
    out.links = []
    for (const l of S.crmLinks) {
      if (l.b === p.id) { const x = S.crm.find(y => y.id === l.a); if (x) out.links.push(relOf(x, l.role === 'via' ? 'via' : 'supplier')) }
      if (l.a === p.id) { const x = S.crm.find(y => y.id === l.b); if (x) out.links.push(relOf(x, l.role === 'via' ? 'forwarded' : 'buyer')) }
    }
    if (!out.links.length) delete out.links
    if (!out.contacts.length) delete out.contacts
  } else if (p.companyId) {
    const co = S.crm.find(x => x.id === p.companyId)
    if (co) out.companies = [relOf(co, '')].map(r => { delete r.role; return r })
  }
  const kindOf = f => f === 'invoice' ? 'invoice' : f === 'contract' ? 'contract' : f === 'po' || f === 'offer' ? 'requisites' : ''
  const files = []
  for (const m of ls) for (const a of m.atts) files.push({ account: m.account, folder: m.folder, uid: m.uid, part: a.part, name: a.name, mime: a.mime, size: a.size, kind: kindOf(a.file), sentAt: iso(m.date), preview: a.preview })
  files.sort((a, b) => b.sentAt.localeCompare(a.sentAt))
  if (files.length) out.files = files
  const hist = ls.slice().sort((a, b) => b.date - a.date).map(m => ({ account: m.account, folder: m.folder, uid: m.uid, messageId: m.messageId, subject: m.subject, sentAt: iso(m.date) }))
  if (hist.length) out.history = hist
  return out
}

// ---------- server-sent events ----------
const clients = new Set()
function publish(ev) {
  const data = `data: ${JSON.stringify(ev)}\n\n`
  for (const c of clients) { try { c.write(data) } catch {} }
}
function dueReminder() {
  const e = S.X.due
  if (!e || S.reminderDone.has(e.id) || (S.snoozed[e.id] && S.snoozed[e.id] > Date.now())) return null
  return e
}
setInterval(() => publish({ type: 'ping' }), 25000).unref()
// A snoozed reminder comes back after its time.
setInterval(() => {
  for (const [id, t] of Object.entries(S.snoozed)) {
    if (t && t <= Date.now()) { delete S.snoozed[id]; const e = S.events.find(x => x.id === Number(id)); if (e && !S.reminderDone.has(e.id)) publish({ type: 'reminder', event: e }) }
  }
}, 5000).unref()

function pushIncoming(spec) {
  const sp = { ...spec, id: spec.id + '-' + (++S.pushed), at: new Date(), seen: false }
  if (sp.re && !S.byId[sp.re]) delete sp.re
  const m = addLetter(sp)
  const sum = summary(m)
  if (spec.filed) publish({ type: 'filed', account: m.account, message: sum })
  else publish({ type: 'new', account: m.account, message: sum })
  publish({ type: 'counts', account: m.account })
  return m
}

// ---------- HTTP plumbing ----------
function send(res, status, body, headers = {}) {
  res.writeHead(status, { 'Cache-Control': 'no-store', ...headers })
  res.end(body)
}
function json(res, v, status = 200) { send(res, status, JSON.stringify(v), { 'Content-Type': 'application/json; charset=utf-8' }) }
function err(res, status, msg) { json(res, { error: msg }, status) }
async function rawBody(req, limit = 60 << 20) {
  const chunks = []
  let n = 0
  for await (const c of req) { n += c.length; if (n > limit) throw new Error('too large'); chunks.push(c) }
  return Buffer.concat(chunks)
}
async function jsonBody(req) {
  const b = await rawBody(req, 4 << 20)
  if (!b.length) return {}
  try { return JSON.parse(b.toString('utf8')) } catch { return {} }
}
// multipart/form-data → { fields: {name: [values]}, files: {name: [{filename, type, data}]} }
async function formBody(req) {
  const ct = req.headers['content-type'] || ''
  const m = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(ct)
  const buf = await rawBody(req)
  const out = { fields: {}, files: {} }
  if (!m) return out
  const boundary = Buffer.from('--' + (m[1] || m[2]))
  let pos = buf.indexOf(boundary)
  while (pos >= 0) {
    const start = pos + boundary.length
    if (buf.slice(start, start + 2).toString() === '--') break
    const next = buf.indexOf(boundary, start)
    if (next < 0) break
    const part = buf.slice(start + 2, next - 2) // skip CRLF after boundary and before the next
    const hEnd = part.indexOf('\r\n\r\n')
    const head = part.slice(0, hEnd).toString('utf8')
    const data = part.slice(hEnd + 4)
    const name = /name="([^"]*)"/i.exec(head)?.[1] || ''
    const filename = /filename="([^"]*)"/i.exec(head)?.[1]
    const type = /content-type:\s*([^\r\n]+)/i.exec(head)?.[1] || 'application/octet-stream'
    if (filename !== undefined) (out.files[name] ||= []).push({ filename, type, data })
    else (out.fields[name] ||= []).push(data.toString('utf8'))
    pos = next
  }
  return out
}
const field = (f, k) => (f.fields[k] || [''])[0]
function parseCookies(req) {
  const out = {}
  for (const p of (req.headers.cookie || '').split(';')) { const i = p.indexOf('='); if (i > 0) out[p.slice(0, i).trim()] = decodeURIComponent(p.slice(i + 1).trim()) }
  return out
}
const signedIn = req => AUTO_LOGIN || !!parseCookies(req).zm_sess

// Address lists typed in the composer: "Name <a@b>, c@d".
function parseAddrs(s) {
  const out = []
  for (const part of String(s || '').split(/[,;](?=(?:[^"]*"[^"]*")*[^"]*$)/)) {
    const t = part.trim()
    if (!t) continue
    const m = /^(.*?)<([^>]+)>$/.exec(t)
    if (m) out.push({ name: m[1].trim().replace(/^"|"$/g, ''), email: lower(m[2].trim()) })
    else if (t.includes('@')) {
      const known = Object.values(S.W.P).find(p => lower(p.email) === lower(t))
      out.push({ name: known ? known.name : '', email: lower(t) })
    }
  }
  return out
}

// ---------- routes ----------
const routes = []
const route = (method, pattern, fn, open = false) => {
  const keys = []
  const re = new RegExp('^' + pattern.replace(/\{(\w+)\}/g, (_, k) => { keys.push(k); return '([^/]+)' }) + '$')
  routes.push({ method, re, keys, fn, open })
}

// --- session ---
route('POST', '/api/auth/login', async (req, res) => {
  const b = await jsonBody(req)
  if (!String(b.login || '').trim() || !b.password) return err(res, 401, 'Wrong username or password')
  send(res, 200, JSON.stringify({ ok: true }), { 'Content-Type': 'application/json', 'Set-Cookie': `zm_sess=demo-${crypto.randomBytes(8).toString('hex')}; Path=/; HttpOnly; SameSite=Lax; Max-Age=2592000` })
}, true)
route('POST', '/api/auth/logout', (req, res) => send(res, 200, JSON.stringify({ ok: true }), { 'Content-Type': 'application/json', 'Set-Cookie': 'zm_sess=; Path=/; Max-Age=0' }))
route('GET', '/api/locale', (req, res) => json(res, { lang: S.me.lang || LANG, timezone: S.W.org.timezone, country: S.W.org.country, org: S.W.org.name }), true)
route('GET', '/api/me', (req, res) => {
  const accs = accounts().map(a => ({ ...a, signature: a.signature.replace(/\{\{ORIGIN\}\}/g, originOf(req)) }))
  json(res, { login: 'anna', name: S.W.U.anna.name, isAdmin: true, accounts: accs, imagePolicy: S.me.imagePolicy, imageSenders: S.me.imageSenders,
    background: S.me.background, backgroundVersion: S.me.backgroundVersion, aiName: 'Loggi', lang: S.me.lang, timezone: S.me.timezone,
    defaults: { lang: LANG, timezone: S.W.org.timezone, country: S.W.org.country, org: S.W.org.name } })
})
route('PUT', '/api/me/locale', async (req, res) => { const b = await jsonBody(req); S.me.lang = b.lang ?? S.me.lang; S.me.timezone = b.timezone ?? S.me.timezone; json(res, { ok: true }) })
route('POST', '/api/me/images', async (req, res) => {
  const b = await jsonBody(req)
  if (b.policy === 'ask' || b.policy === 'always') S.me.imagePolicy = b.policy
  if (b.allow && !S.me.imageSenders.includes(lower(b.allow))) S.me.imageSenders.push(lower(b.allow))
  if (b.remove) S.me.imageSenders = S.me.imageSenders.filter(x => x !== lower(b.remove))
  S.me.imageSenders.sort()
  json(res, { imagePolicy: S.me.imagePolicy, imageSenders: S.me.imageSenders })
})
route('POST', '/api/me/avatar', async (req, res) => {
  const f = await formBody(req)
  const p = (f.files.photo || [])[0]
  if (!p) return err(res, 400, 'No picture')
  S.photos.anna = { type: p.type, data: p.data }
  json(res, { ok: true, version: Math.floor(Date.now() / 1000) })
})
route('DELETE', '/api/me/avatar', (req, res) => { delete S.photos.anna; json(res, { ok: true }) })
route('POST', '/api/me/background', async (req, res) => {
  const b = await jsonBody(req)
  S.me.background = String(b.background || '')
  json(res, { background: S.me.background, backgroundVersion: S.me.backgroundVersion })
})
route('POST', '/api/me/background/upload', async (req, res) => {
  const f = await formBody(req)
  const p = (f.files.photo || [])[0]
  if (!p) return err(res, 400, 'No picture')
  S.bgImage = { type: p.type, data: p.data }
  S.me.background = 'custom'
  S.me.backgroundVersion = Math.floor(Date.now() / 1000)
  json(res, { background: S.me.background, backgroundVersion: S.me.backgroundVersion })
})
route('GET', '/api/me/background/image', (req, res) => {
  const img = S.bgImage || { type: 'image/jpeg', data: asset('common/background.jpg') }
  send(res, 200, img.data, { 'Content-Type': img.type, 'Cache-Control': 'private, max-age=86400' })
})
route('GET', '/api/settings', (req, res) => json(res, S.settings))
route('PUT', '/api/settings', async (req, res) => { Object.assign(S.settings, await jsonBody(req)); json(res, { ok: true }) })
route('PUT', '/api/mailbox-settings', async (req, res) => {
  const b = await jsonBody(req)
  if (!accountOf(b.address)) return err(res, 403, 'The signature of this mailbox is set by the administrator')
  S.W.signatures[lower(b.address)] = String(b.signature || '').replace(/\s+$/, '')
  S.W.startTemplates[lower(b.address)] = String(b.template || '')
  json(res, { ok: true })
})
route('GET', '/api/identities', (req, res) => json(res, [{ address: S.W.gmail, name: S.W.U.anna.name, mailbox: S.W.gmail }]))
route('GET', '/api/quota', (req, res) => json(res, accounts().map(a => {
  const mb = mailboxByAddr(a.address)
  return { address: a.address, used: Math.round(mb.usedMB * 2 ** 20), messages: mb.messages, limit: mb.quotaGB * 2 ** 30 }
})))

// --- folders ---
function folderList(acct) {
  return folderNames(acct).map(name => {
    const ms = S.msgs.filter(m => m.account === acct && m.folder === name)
    return { name, role: FOLDER_ROLE[name] || '', unseen: ms.filter(m => !m.seen).length, total: ms.length }
  }).sort((a, b) => (ROLE_ORDER[a.role] - ROLE_ORDER[b.role]) || a.name.localeCompare(b.name))
}
route('GET', '/api/folders', (req, res) => json(res, { accounts: accounts().map(a => ({ account: a.address, folders: folderList(a.address) })) }))
route('GET', '/api/folders/rules', (req, res, q) => json(res, S.X.folderRules.filter(r => r.account === lower(q.account))))
async function saveFolder(req, res, create) {
  const b = await jsonBody(req)
  const acct = lower(b.account)
  if (!accountOf(acct)) return err(res, 403, 'No access to this mailbox')
  const list = (S.X.ownFolders[acct] ||= [])
  const name = String(create ? b.folder : b.rename || b.folder).trim()
  if (!name) return err(res, 400, 'Name the folder')
  if (create) {
    if (folderNames(acct).includes(name)) return err(res, 409, 'A folder with this name already exists')
    list.push(name)
  } else if (b.rename && b.rename !== b.folder) {
    const i = list.indexOf(b.folder)
    if (i >= 0) list[i] = name
    for (const m of S.msgs) if (m.account === acct && m.folder === b.folder) m.folder = name
    for (const r of S.X.folderRules) if (r.account === acct && r.folder === b.folder) r.folder = name
  }
  const senders = String((b.senders || []).join('\n')).split(/[\s,;]+/).map(lower).filter(Boolean)
  let r = S.X.folderRules.find(x => x.account === acct && x.folder === name)
  if (!r) { r = { account: acct, folder: name }; S.X.folderRules.push(r) }
  Object.assign(r, { description: b.description || '', senders, auto: !!b.auto, hideInbox: !!b.hideInbox, expireDays: Number(b.expireDays) || 0 })
  json(res, { ok: true })
}
route('POST', '/api/folders', (req, res) => saveFolder(req, res, true))
route('PUT', '/api/folders', (req, res) => saveFolder(req, res, false))
route('DELETE', '/api/folders', (req, res, q) => {
  const acct = lower(q.account)
  const list = S.X.ownFolders[acct] || []
  const i = list.indexOf(q.folder)
  if (i < 0) return err(res, 404, 'Folder not found')
  list.splice(i, 1)
  for (const m of S.msgs) if (m.account === acct && m.folder === q.folder) { m.folder = 'Trash'; m.uid = nextUid(acct, 'Trash') }
  S.X.folderRules = S.X.folderRules.filter(r => !(r.account === acct && r.folder === q.folder))
  json(res, { ok: true })
})
route('POST', '/api/folders/sender', async (req, res) => {
  const b = await jsonBody(req)
  const acct = lower(b.account)
  let r = S.X.folderRules.find(x => x.account === acct && x.folder === b.folder)
  if (!r) { r = { account: acct, folder: b.folder, description: '', senders: [], auto: true, hideInbox: false, expireDays: 0 }; S.X.folderRules.push(r) }
  const s = lower(b.sender)
  if (!r.senders.includes(s)) r.senders.push(s)
  // The sender's other letters in the Inbox follow.
  for (const m of S.msgs) {
    if (m.account === acct && m.folder === 'INBOX' && (s.startsWith('@') ? m.from.email.endsWith(s) : m.from.email === s)) { m.folder = b.folder; m.uid = nextUid(acct, b.folder) }
  }
  json(res, { ok: true })
})

// --- message list, search, a letter, a conversation ---
function matches(m, text) {
  const t = lower(text)
  const hay = lower([m.subject, m.from.name, m.from.email, ...m.to.map(a => a.name + ' ' + a.email), ...m.cc.map(a => a.name + ' ' + a.email), m.text, ...m.atts.map(a => a.name)].join('\n'))
  return t.split(/\s+/).filter(Boolean).every(w => hay.includes(w))
}
route('GET', '/api/messages', async (req, res, q) => {
  const acct = lower(q.account)
  if (!accountOf(acct)) return err(res, 403, 'No access to this mailbox')
  const limit = Math.min(200, Number(q.limit) || 50)
  const before = Number(q.before) || 0
  const since = q.since ? new Date(q.since + 'T00:00:00') : null
  const until = q.until ? new Date(q.until + 'T23:59:59') : null
  const base = m => (!since || m.date >= since) && (!until || m.date <= until) && (q.unseen !== '1' || !m.seen) && (q.flagged !== '1' || m.flagged)
  const text = String(q.q || '').trim()
  if (text) await sleep(250) // searching takes a moment on a real server too
  if (Number(q.label) > 0) {
    const id = Number(q.label)
    const list = S.msgs.filter(m => m.account === acct && !['Trash', 'Junk', 'Drafts'].includes(m.folder) && m.labels.includes(id) && base(m) && (!text || matches(m, text)))
      .sort((a, b) => b.date - a.date).slice(0, limit)
    return json(res, { folder: '', messages: list.map(summary), hasMore: false })
  }
  const folder = resolveFolder(acct, q)
  if (!folder || !folderNames(acct).includes(folder)) return err(res, 404, 'Folder not found')
  if (before === 1) return json(res, { folder, messages: [], hasMore: false })
  let list = S.msgs.filter(m => m.account === acct && m.folder === folder && base(m) && (!before || m.uid < before) && (!text || matches(m, text)))
  list.sort((a, b) => b.uid - a.uid)
  const hasMore = list.length > limit
  list = list.slice(0, limit)
  json(res, { folder, messages: list.map(summary), hasMore, partial: false, indexed: !!text })
})
route('GET', '/api/message', (req, res, q) => {
  const acct = lower(q.account)
  if (!accountOf(acct)) return err(res, 403, 'No access to this mailbox')
  const folder = resolveFolder(acct, q)
  const m = find(acct, folder, q.uid)
  if (!m) return err(res, 404, 'Message not found. It may have been moved or deleted')
  if (!m.seen && q.peek !== '1') { m.seen = true; publish({ type: 'counts', account: acct }) }
  json(res, fullMsg(m, req, q.text === '1'))
})
route('GET', '/api/thread', (req, res, q) => {
  const acct = lower(q.account)
  const folder = resolveFolder(acct, q)
  const m = find(acct, folder, q.uid)
  if (!m) return err(res, 404, 'Message not found')
  const where = new Set(['INBOX', 'Sent', 'Archive', folder])
  const all = S.msgs.filter(x => x.account === acct && x.root === m.root && where.has(x.folder)).sort((a, b) => a.date - b.date)
  json(res, { messages: (all.length ? all : [m]).map(summary) })
})
route('POST', '/api/messages/action', async (req, res) => {
  const b = await jsonBody(req)
  const acct = lower(b.account)
  if (!accountOf(acct)) return err(res, 403, 'No access to this mailbox')
  if (!b.uids?.length || !b.folder) return err(res, 400, 'No messages selected')
  const list = b.uids.map(u => find(acct, b.folder, u)).filter(Boolean)
  for (const m of list) {
    switch (b.action) {
      case 'seen': m.seen = true; break
      case 'unseen': m.seen = false; break
      case 'flag': m.flagged = true; break
      case 'unflag': m.flagged = false; break
      case 'label': if (!m.labels.includes(Number(b.target))) m.labels.push(Number(b.target)); break
      case 'unlabel': m.labels = m.labels.filter(x => x !== Number(b.target)); break
      case 'move': case 'delete': {
        const target = b.action === 'delete' ? 'trash' : String(b.target || '')
        const dest = target.startsWith('folder:') ? target.slice(7) : ROLE_FOLDER[target]
        if (!dest || !folderNames(acct).includes(dest)) return err(res, 400, 'No such folder')
        if (b.action === 'delete' && dest === m.folder) { S.msgs.splice(S.msgs.indexOf(m), 1); break }
        if (dest !== m.folder) { m.folder = dest; m.uid = nextUid(acct, dest); if (dest === 'Junk') m.spam = true; if (dest === 'INBOX') m.spam = false }
        break
      }
      default: return err(res, 400, 'Unknown action')
    }
  }
  publish({ type: 'counts', account: acct })
  json(res, { ok: true })
})

// --- attachments ---
function attOf(q) {
  const acct = lower(q.account)
  if (!accountOf(acct)) return {}
  const m = find(acct, resolveFolder(acct, q), q.uid)
  if (!m) return {}
  return { m, a: m.atts.find(a => a.part === q.part) }
}
function attBytes(m, a) { return a.file ? fileBytes(S.files[a.file]) : (m.extraFiles[a.part]?.data || Buffer.alloc(0)) }
function disposition(kind, name) { return `${kind}; filename="${name.replace(/[^\x20-\x7e]/g, '_').replace(/"/g, '')}"; filename*=UTF-8''${encodeURIComponent(name)}` }
route('GET', '/api/attachment', async (req, res, q) => {
  const { m, a } = attOf(q)
  if (!m) return err(res, 404, 'Message not found')
  if (!a) return err(res, 404, 'Attachment not found')
  const f = a.file ? S.files[a.file] : null
  const as = q.as || ''
  if (as === 'scan') {
    await sleep(300)
    return f?.virus ? json(res, { status: 'virus', virus: f.virus, kind: 'virus' }) : json(res, { status: 'clean', virus: '', kind: '' })
  }
  if (f?.virus && as !== 'thumb') return err(res, 403, `A virus was found in the file (${f.virus}). It cannot be downloaded`)
  if (as === 'thumb') {
    if (a.preview === 'image') return send(res, 200, attBytes(m, a), { 'Content-Type': a.mime, 'Cache-Control': 'private, max-age=3600' })
    if (f?.thumb) return send(res, 200, asset(f.thumb), { 'Content-Type': 'image/png', 'Cache-Control': 'private, max-age=3600' })
    return send(res, 404, '')
  }
  if (as === 'text') {
    if (a.preview !== 'text') return err(res, 400, 'This file cannot be shown as text')
    return send(res, 200, attBytes(m, a), { 'Content-Type': 'text/plain; charset=utf-8' })
  }
  if (as === 'html') {
    if (a.preview !== 'sheet' || !f?.html) return err(res, 400, 'This file cannot be shown as a spreadsheet')
    await sleep(500)
    return send(res, 200, asset(f.html), { 'Content-Type': 'text/html; charset=utf-8' })
  }
  if (as === 'pdf') {
    if (!f?.pdf) return err(res, 400, 'This document cannot be shown')
    await sleep(600)
    return send(res, 200, asset(f.pdf), { 'Content-Type': 'application/pdf', 'Content-Disposition': disposition('inline', a.name.replace(/\.\w+$/, '') + '.pdf') })
  }
  if (as === 'ocr') {
    if (a.preview !== 'image' && a.preview !== 'pdf') return err(res, 400, 'Text cannot be recognized in this file')
    await sleep(700)
    if (f?.ocr) return send(res, 200, asset(f.ocr), { 'Content-Type': 'application/json' })
    const [w, h] = f?.ocrEmpty || [1000, 1000]
    return json(res, { pages: [{ w, h, words: [] }], text: '' })
  }
  if (as === 'page') {
    if (f?.page) return send(res, 200, asset(f.page), { 'Content-Type': 'image/jpeg' })
    return err(res, 422, 'Could not show the page')
  }
  const data = attBytes(m, a)
  const inline = a.inline && q.download !== '1'
  send(res, 200, data, { 'Content-Type': inline ? a.mime : 'application/octet-stream', 'Content-Disposition': disposition(inline ? 'inline' : 'attachment', a.name), 'Cache-Control': 'private, max-age=3600' })
})
route('GET', '/api/cid', (req, res, q) => {
  const acct = lower(q.account)
  const m = find(acct, resolveFolder(acct, q), q.uid)
  const c = m && m.cids[q.cid]
  if (!c) return err(res, 404, 'Image not found')
  send(res, 200, c.data, { 'Content-Type': c.type, 'Cache-Control': 'private, max-age=3600' })
})
route('GET', '/api/attachments.zip', (req, res, q) => {
  const acct = lower(q.account)
  const m = find(acct, resolveFolder(acct, q), q.uid)
  if (!m) return err(res, 404, 'Message not found')
  const files = {}
  for (const a of m.atts) if (!(a.file && S.files[a.file].virus)) files[a.name] = attBytes(m, a)
  const name = (m.subject || 'attachments').replace(/[\\/:*?"<>|]+/g, '_').slice(0, 60) + '.zip'
  send(res, 200, zip(files), { 'Content-Type': 'application/zip', 'Content-Disposition': disposition('attachment', name) })
})

// --- compose: send, drafts, large files ---
async function composeIn(req) {
  const f = await formBody(req)
  const from = lower(field(f, 'from'))
  const ident = from === S.W.gmail ? S.W.gmail : from
  const acc = accountOf(ident)
  return { f, from, acc }
}
function letterFromForm(f, from, folder, acct, extra = {}) {
  const parts = []
  const extraFiles = {}
  let n = 2
  // Files kept from the previous draft or the forwarded letter.
  const carryUid = Number(field(f, 'carryUid'))
  if (carryUid) {
    const src = find(lower(field(f, 'carryAccount')), field(f, 'carryFolder'), carryUid)
    const keep = new Set(field(f, 'carryParts').split(',').filter(Boolean))
    for (const a of src?.atts || []) if (keep.has(a.part)) {
      const p = String(n++)
      parts.push({ ...a, part: p })
      if (!a.file) extraFiles[p] = src.extraFiles[a.part]
    }
    for (const cid of field(f, 'carryCids').split(',').filter(Boolean)) if (src?.cids[cid]) extra.cids = { ...(extra.cids || {}), [cid]: src.cids[cid] }
  }
  for (const file of f.files.files || []) {
    const p = String(n++)
    const mime = file.type && file.type !== 'application/octet-stream' ? file.type : mimeByName(file.filename)
    parts.push({ part: p, name: file.filename, mime, size: file.data.length, inline: inlineSafe(mime), preview: previewKind(mime, file.filename), risk: riskOf(file.filename) })
    extraFiles[p] = { data: file.data }
  }
  const cids = { ...(extra.cids || {}) }
  const inl = f.files.inline || []
  const inlCid = f.fields.inlineCid || []
  inl.forEach((file, i) => { cids[inlCid[i]] = { type: file.type, data: file.data } })
  const html = field(f, 'html')
  const id = 'c' + (++S.seq)
  const sp = { id, acct, folder, from, to: parseAddrs(field(f, 'to')), cc: parseAddrs(field(f, 'cc')), bcc: parseAddrs(field(f, 'bcc')), at: new Date(), seen: true,
    subject: field(f, 'subject'), html, text: field(f, 'text'), cids, replyFolder: field(f, 'replyFolder'), replyUid: field(f, 'replyUid'), extraFiles, ...extra }
  const m = addLetter({ ...sp, text: undefined })
  m.text = field(f, 'text')
  m.preview = previewOf(m.text.replace(/^>.*$/gm, ''))
  m.atts = parts
  m.size = Math.round(html.length * 1.1 + parts.reduce((s, a) => s + a.size * 1.37, 0)) + 2400
  // A reply joins the conversation it answers.
  const inReplyTo = field(f, 'inReplyTo')
  if (inReplyTo) {
    const orig = S.msgs.find(x => x.messageId === inReplyTo)
    m.inReplyTo = inReplyTo
    m.refs = field(f, 'references').split(/\s+/).filter(Boolean)
    if (!m.refs.includes(inReplyTo)) m.refs.push(inReplyTo)
    if (orig) m.root = orig.root
  }
  return m
}
function dropDraft(acct, uid) {
  const d = find(acct, 'Drafts', uid)
  if (d) S.msgs.splice(S.msgs.indexOf(d), 1)
}
route('POST', '/api/drafts', async (req, res) => {
  const { f, from, acc } = await composeIn(req)
  if (!acc) return err(res, 403, 'You cannot send from this address')
  const m = letterFromForm(f, from, 'Drafts', acc.address)
  const prevUid = Number(field(f, 'draftUid'))
  if (prevUid) dropDraft(lower(field(f, 'draftAccount')), prevUid)
  publish({ type: 'counts', account: acc.address })
  json(res, { account: m.account, folder: m.folder, uid: m.uid, attachments: m.atts.map(attOut), savedAt: iso(new Date()) })
})
route('POST', '/api/drafts/delete', async (req, res) => {
  const b = await jsonBody(req)
  const d = find(lower(b.account), 'Drafts', b.uid)
  if (d) { d.folder = 'Trash'; d.uid = nextUid(d.account, 'Trash') }
  json(res, { ok: true })
})
route('POST', '/api/send', async (req, res) => {
  const { f, from, acc } = await composeIn(req)
  if (!acc) return err(res, 403, 'You cannot send from this address')
  const shared = !mailboxByAddr(acc.address)?.users.find(u => u[0] === 1 && u[2])
  const m = letterFromForm(f, from, 'Sent', acc.address, shared && !mailboxByAddr(acc.address)?.external ? { by: 'anna' } : {})
  if (shared && !mailboxByAddr(acc.address)?.external) m.sentBy = { login: 'anna', name: S.W.U.anna.name }
  const draftUid = Number(field(f, 'draftUid'))
  if (draftUid) dropDraft(lower(field(f, 'draftAccount')), draftUid)
  const ru = Number(field(f, 'replyUid'))
  if (ru) { const o = find(acc.address, field(f, 'replyFolder'), ru); if (o) o.answered = true }
  if (!m.to.length && !m.cc.length && !m.bcc.length) return err(res, 400, 'Specify at least one recipient')
  S.X.sentLog.unshift({ id: 1000 + S.seq, at: iso(new Date()), login: 'anna', mailbox: acc.address, subject: m.subject, recipients: [...m.to, ...m.cc, ...m.bcc].map(a => a.email) })
  // A letter to one of our own mailboxes arrives there a moment later.
  for (const r of [...m.to, ...m.cc, ...m.bcc]) {
    if (accountOf(r.email) && r.email !== acc.address) {
      setTimeout(() => {
        const copy = { ...m, id: m.id + '-in-' + r.email, account: r.email, folder: 'INBOX', seen: false, sentBy: null, auth: {}, atts: m.atts.map(a => ({ ...a })), labels: [] }
        copy.uid = nextUid(r.email, 'INBOX')
        S.msgs.push(copy)
        publish({ type: 'new', account: r.email, message: summary(copy) })
      }, 2500)
    }
  }
  publish({ type: 'counts', account: acc.address })
  json(res, { ok: '1', messageId: m.messageId, account: m.account, folder: m.folder, uid: m.uid })
})
route('POST', '/api/bigfiles', async (req, res, q) => {
  const data = await rawBody(req, 3 << 30)
  const token = crypto.randomBytes(9).toString('base64url')
  const name = q.name || 'file'
  const mime = req.headers['content-type'] || mimeByName(name)
  const img = /^image\/(png|jpeg|gif|webp)$/.test(mime)
  const fileInfo = { token, url: `${originOf(req)}/f/${token}`, name, size: data.length, mime, expires: iso(new Date(Date.now() + 30 * 86400000)) }
  if (img) fileInfo.thumb = `${originOf(req)}/t/${token}.jpg`
  S.bigfiles[token] = { ...fileInfo, data, sender: q.mailbox || '' }
  json(res, fileInfo)
})
route('DELETE', '/api/bigfiles/{token}', (req, res, q, p) => { delete S.bigfiles[p.token]; json(res, { ok: true }) })
route('POST', '/api/signature-image', async (req, res) => {
  const data = await rawBody(req, 2 << 20)
  const type = req.headers['content-type'] || 'image/png'
  const name = crypto.randomBytes(8).toString('hex') + ({ 'image/jpeg': '.jpg', 'image/gif': '.gif', 'image/webp': '.webp' }[type] || '.png')
  S.sigImages[name] = { type, data }
  json(res, { url: `${originOf(req)}/i/${name}` })
})

// --- contacts, labels, templates, auto-reply, sender rules, spelling ---
route('GET', '/api/contacts', (req, res, q) => {
  const t = lower(q.q).trim()
  const stats = {}
  for (const m of S.msgs) {
    const out = !!accountOf(m.from.email)
    for (const a of out ? [...m.to, ...m.cc] : [m.from]) {
      if (!a.email || accountOf(a.email)) continue
      const s = (stats[a.email] ||= { email: a.email, name: a.name, sent: 0, recv: 0, last: m.date })
      if (out) s.sent++; else s.recv++
      if (m.date > s.last) s.last = m.date
      if (!s.name && a.name) s.name = a.name
    }
  }
  for (const p of Object.values(S.W.P)) if (p.email && !stats[lower(p.email)] && !accountOf(p.email)) stats[lower(p.email)] = { email: lower(p.email), name: p.name, sent: 0, recv: 0, last: S.now }
  const list = Object.values(stats).map(s => {
    const pc = S.crm.find(x => x.kind === 'person' && x.emails.includes(s.email))
    const co = pc?.companyId ? S.crm.find(x => x.id === pc.companyId) : S.crm.find(x => x.kind === 'company' && x.domains.includes(s.email.split('@')[1]))
    return { email: s.email, name: s.name, sent: s.sent, recv: s.recv, last: iso(s.last), ...(co ? { company: co.name } : {}), ...(pc?.position ? { position: pc.position } : {}) }
  }).filter(h => !t || lower(h.name + ' ' + h.email + ' ' + (h.company || '')).includes(t))
    .sort((a, b) => (b.sent * 3 + b.recv) - (a.sent * 3 + a.recv) || b.last.localeCompare(a.last))
  json(res, list.slice(0, 20))
})
route('GET', '/api/labels', (req, res) => json(res, S.X.labels))
route('POST', '/api/labels', async (req, res) => { const b = await jsonBody(req); const l = { id: Math.max(0, ...S.X.labels.map(x => x.id)) + 1, name: String(b.name || '').trim(), color: b.color || 'blue' }; if (!l.name) return err(res, 400, 'Name the label'); S.X.labels.push(l); json(res, l) })
route('PUT', '/api/labels/{id}', async (req, res, q, p) => { const b = await jsonBody(req); const l = S.X.labels.find(x => x.id === Number(p.id)); if (!l) return err(res, 404, 'Label not found'); Object.assign(l, { name: b.name ?? l.name, color: b.color ?? l.color }); json(res, l) })
route('DELETE', '/api/labels/{id}', (req, res, q, p) => { S.X.labels = S.X.labels.filter(x => x.id !== Number(p.id)); for (const m of S.msgs) m.labels = m.labels.filter(x => x !== Number(p.id)); json(res, { ok: true }) })
route('GET', '/api/templates', (req, res) => json(res, S.X.templates))
route('POST', '/api/templates', async (req, res) => { const b = await jsonBody(req); const t = { id: Math.max(0, ...S.X.templates.map(x => x.id)) + 1, title: b.title || '', body: b.body || '', mailbox: b.mailbox || '', shared: !!b.shared, mine: true }; S.X.templates.push(t); json(res, t) })
route('PUT', '/api/templates/{id}', async (req, res, q, p) => { const b = await jsonBody(req); const t = S.X.templates.find(x => x.id === Number(p.id)); if (!t) return err(res, 404, 'Template not found'); Object.assign(t, { title: b.title ?? t.title, body: b.body ?? t.body, mailbox: b.mailbox ?? t.mailbox, shared: b.shared ?? t.shared }); json(res, { ok: true }) })
route('DELETE', '/api/templates/{id}', (req, res, q, p) => { S.X.templates = S.X.templates.filter(x => x.id !== Number(p.id)); json(res, { ok: true }) })
route('GET', '/api/autoreply', (req, res, q) => {
  const mb = lower(q.mailbox)
  json(res, S.X.autoreply[mb] || { mailbox: mb, enabled: false, reason: 'vacation', dateFrom: '', dateTo: '', substitute: '', notes: '', subject: '', body: '' })
})
route('PUT', '/api/autoreply', async (req, res) => { const b = await jsonBody(req); S.X.autoreply[lower(b.mailbox)] = { ...b, mailbox: lower(b.mailbox), updatedBy: 'anna' }; json(res, { ok: true }) })
route('POST', '/api/autoreply/draft', async (req, res) => { const b = await jsonBody(req); await sleep(1200); json(res, autoReplyText(LANG, b)) })
function addSenderRules(b) {
  let added = 0
  const invalid = []
  for (const raw of (b.entries || []).join('\n').split(/[\n,;]+/)) {
    const t = raw.trim()
    if (!t) continue
    const m = /^(.*?)<([^>]+)>$/.exec(t)
    const pattern = lower(m ? m[2] : t).trim()
    if (!/^(@[a-z0-9.-]+\.[a-z]{2,}|[^@\s]+@[a-z0-9.-]+\.[a-z]{2,})$/.test(pattern)) { invalid.push(t); continue }
    S.X.senderRules = S.X.senderRules.filter(r => r.pattern !== pattern)
    S.X.senderRules.unshift({ id: Math.max(0, ...S.X.senderRules.map(x => x.id)) + 1, kind: b.kind === 'allow' ? 'allow' : 'block', pattern, name: m ? m[1].trim().replace(/^"|"$/g, '') : '', note: b.note || '', createdBy: 'anna', createdAt: iso(new Date()) })
    added++
  }
  return { added, invalid }
}
route('POST', '/api/sender-rules', async (req, res) => json(res, addSenderRules(await jsonBody(req))))
const BAD = {
  recieve: ['receive'], recieved: ['received'], adress: ['address'], seperate: ['separate'], untill: ['until'], tommorow: ['tomorrow'], wich: ['which'], definately: ['definitely'], teh: ['the'], occured: ['occurred'], accomodate: ['accommodate'], beleive: ['believe'],
  'пожалуйсто': ['пожалуйста'], 'извените': ['извините'], 'зделать': ['сделать'], 'сдесь': ['здесь'], 'обезательно': ['обязательно'], 'прийдёт': ['придёт'], 'прийдет': ['придёт'], 'вобщем': ['в общем'], 'расчитать': ['рассчитать'], 'симпатишный': ['симпатичный'],
}
// The dictionary: every word of the demo's letters plus a few common ones. A word is reported when
// it is a known typo, or when it looks like a letter moved across a space ("thisf eature").
const EXTRA_WORDS = 'this feature features function the a an and or our your we you will next week new portal works available please find attached этот эта это функционал функционала работает будет на следующей неделе наш новый портал доступен мы вы'.split(' ')
let DICT = null
function dict() {
  if (DICT) return DICT
  DICT = new Set(EXTRA_WORDS)
  for (const m of S.msgs) for (const w of (m.subject + ' ' + m.text).toLowerCase().match(/\p{L}+/gu) || []) DICT.add(w)
  return DICT
}
function badWord(w) {
  const lw = lower(w)
  if (BAD[lw]) return BAD[lw]
  const d = dict()
  if (d.has(lw) || lw.length < 3 || S.spellDict.has(lw)) return null
  if (d.has(lw.slice(0, -1)) || d.has(lw.slice(1))) return []
  for (const x of d) if (x.length === lw.length + 1 && (x.endsWith(lw) || x.startsWith(lw))) return []
  return null
}
route('POST', '/api/spell', async (req, res) => {
  const b = await jsonBody(req)
  const bad = {}
  const pairs = {}
  for (const w of b.words || []) { const s = badWord(w); if (s) bad[w] = s }
  for (const p of b.pairs || []) {
    const [a, c] = p.split(' ')
    const j = a + c
    const fixes = []
    for (let i = 1; i < j.length; i++) {
      const x = j.slice(0, i), y = j.slice(i)
      if (x !== a && dict().has(lower(x)) && dict().has(lower(y))) fixes.push(`${x} ${y}`)
    }
    if (fixes.length) pairs[p] = fixes.slice(0, 3)
  }
  json(res, { bad, pairs })
})
route('GET', '/api/spell/dict', (req, res) => json(res, { words: [...S.spellDict].sort() }))
route('POST', '/api/spell/dict', async (req, res) => { const b = await jsonBody(req); S.spellDict.add(lower(b.word)); json(res, { ok: true }) })
route('DELETE', '/api/spell/dict', (req, res, q) => { S.spellDict.delete(lower(q.word)); json(res, { ok: true }) })

// --- avatars ---
const logoColors = ['#0B7A6E', '#B26A12', '#3D5FA8', '#8A4B9C', '#2F7D4F', '#A8433A', '#1F6FB2', '#7A5C2E']
function companyLogo(name, domain) {
  let h = 0
  for (const c of domain) h = (h * 31 + c.charCodeAt(0)) >>> 0
  const col = logoColors[h % logoColors.length]
  const letter = (name.replace(/^(ООО|АО|ТОО|СП ООО|МУП)\s*«?/u, '').trim()[0] || '?').toUpperCase()
  const shape = h % 3 === 0 ? `<circle cx="32" cy="32" r="22" fill="none" stroke="#fff" stroke-width="5" opacity=".35"/>` : h % 3 === 1 ? `<path d="M10 46 L32 14 L54 46Z" fill="#fff" opacity=".22"/>` : `<rect x="14" y="14" width="36" height="36" rx="6" fill="#fff" opacity=".2" transform="rotate(45 32 32)"/>`
  return `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64"><rect width="64" height="64" fill="${col}"/>${shape}<text x="32" y="43" text-anchor="middle" font-family="Arial,Helvetica,sans-serif" font-weight="700" font-size="30" fill="#fff">${esc(letter)}</text></svg>`
}
route('GET', '/api/avatar', (req, res, q) => {
  const email = lower(q.email)
  const own = S.W.users.find(u => u.personal === email)
  if (own && S.photos[own.login]) return send(res, 200, S.photos[own.login].data, { 'Content-Type': S.photos[own.login].type })
  const dom = email.split('@')[1] || ''
  const co = q.bimi === '1' && !PUBLIC_DOMAINS.has(dom) && S.crm.find(x => x.kind === 'company' && !x.own && x.domains.includes(dom))
  if (co) return send(res, 200, companyLogo(co.name, dom), { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'private, max-age=86400' })
  if (dom === 'pumpweekly.example' && q.bimi === '1') return send(res, 200, companyLogo('Pump Weekly', dom), { 'Content-Type': 'image/svg+xml' })
  send(res, 404, '', { 'Cache-Control': 'private, max-age=21600' })
})
route('GET', '/api/user-photo', (req, res, q) => {
  const p = S.photos[lower(q.login)]
  if (!p) return send(res, 404, '', { 'Cache-Control': 'private, max-age=300' })
  send(res, 200, p.data, { 'Content-Type': p.type })
})

// --- calendar ---
route('GET', '/api/calendar/events', (req, res, q) => {
  const from = q.from ? new Date(q.from) : new Date(0)
  const to = q.to ? new Date(q.to) : new Date(8.64e15)
  json(res, S.events.filter(e => new Date(e.start) < to && new Date(e.end || e.start) >= from).map(evOut))
})
const evOut = e => { const o = { ...e }; if (o.end == null) delete o.end; if (o.remindMinutes == null) delete o.remindMinutes; if (!o.mailRef) delete o.mailRef; return o }
function evIn(b, e = {}) {
  return Object.assign(e, { title: String(b.title ?? e.title ?? '').trim(), notes: b.notes ?? e.notes ?? '', start: b.start ? iso(b.start) : e.start, end: b.end ? iso(b.end) : null,
    allDay: !!b.allDay, color: b.color || e.color || 'blue', isTask: !!b.isTask, done: !!b.done, remindMinutes: b.remindMinutes ?? null, mailRef: b.mailRef ?? e.mailRef ?? '' })
}
route('POST', '/api/calendar/events', async (req, res) => {
  const b = await jsonBody(req)
  if (!String(b.title || '').trim() || !b.start) return err(res, 400, 'Name the event and choose its time')
  const e = evIn(b, { id: Math.max(0, ...S.events.map(x => x.id)) + 1 })
  S.events.push(e)
  json(res, evOut(e))
})
route('PUT', '/api/calendar/events/{id}', async (req, res, q, p) => {
  const e = S.events.find(x => x.id === Number(p.id))
  if (!e) return err(res, 404, 'Event not found')
  evIn(await jsonBody(req), e)
  json(res, evOut(e))
})
route('DELETE', '/api/calendar/events/{id}', (req, res, q, p) => { S.events = S.events.filter(x => x.id !== Number(p.id)); json(res, { ok: true }) })
route('POST', '/api/calendar/events/{id}/reminder', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const id = Number(p.id)
  const e = S.events.find(x => x.id === id)
  if (b.action === 'done') { S.reminderDone.add(id); if (e?.isTask) e.done = true; publish({ type: 'reminder_done', event: e || { id } }) }
  else S.snoozed[id] = Date.now() + (Number(b.minutes) || 15) * 60000
  json(res, { ok: true })
})
route('GET', '/api/calendar/plan', (req, res, q) => json(res, S.plan[q.m] || { goals: [], tasks: [], notes: '' }))
route('PUT', '/api/calendar/plan', async (req, res, q) => { const b = await jsonBody(req); S.plan[q.m] = { goals: b.goals || [], tasks: b.tasks || [], notes: b.notes || '' }; json(res, { ok: true }) })

// --- counterparty cards ---
route('GET', '/api/crm/parties', (req, res, q) => {
  const t = lower(q.q).trim()
  const list = S.crm.filter(p => (!q.kind || p.kind === q.kind) && (!t || lower([p.name, p.inn, p.position, p.notes, ...p.emails, ...p.domains, ...p.phones, ...Object.values(p.ids)].join(' ')).includes(t)))
    .map(p => partyOut(p, false))
  list.sort((a, b) => (b.lastLetter || '').localeCompare(a.lastLetter || '') || a.name.localeCompare(b.name))
  json(res, list)
})
route('GET', '/api/crm/parties/{id}', (req, res, q, p) => {
  const x = S.crm.find(y => y.id === Number(p.id))
  if (!x) return err(res, 404, 'Card not found')
  json(res, partyOut(x, true))
})
route('GET', '/api/crm/lookup', (req, res, q) => {
  const email = lower(q.email)
  const person = S.crm.find(x => x.kind === 'person' && x.emails.includes(email))
  let co = person?.companyId ? S.crm.find(x => x.id === person.companyId) : person
  if (!co) {
    const dom = email.split('@')[1] || ''
    if (!PUBLIC_DOMAINS.has(dom)) co = S.crm.find(x => x.kind === 'company' && x.domains.includes(dom))
  }
  if (!co) return send(res, 204, '')
  const out = partyOut(co, false)
  if (person && person !== co) out.contacts = [partyOut(person, false)]
  json(res, out)
})
async function crmSave(req, res, id) {
  const b = await jsonBody(req)
  const name = String(b.name || '').trim()
  if (!name || (b.kind !== 'company' && b.kind !== 'person')) return err(res, 400, 'A company name or a person\'s full name is required')
  let p = id ? S.crm.find(x => x.id === id) : null
  if (id && !p) return err(res, 404, 'Card not found')
  if (!p) { p = { id: ++S.crmNext.party, own: false, addresses: [], suggestions: [], companyId: null, companyName: '' }; S.crm.push(p) }
  const clean = xs => (xs || []).map(x => String(x).trim()).filter(Boolean)
  Object.assign(p, { kind: b.kind, name, country: b.kind === 'company' ? String(b.country || '').toUpperCase() : '', ids: b.kind === 'company' ? (b.ids || {}) : {}, inn: b.inn || '', kpp: b.kpp || '', ogrn: b.ogrn || '',
    legalAddress: b.legalAddress || '', bankName: b.bankName || '', bik: b.bik || '', account: b.account || '', corrAccount: b.corrAccount || '', website: b.website || '', position: b.position || '',
    notes: b.notes || '', phones: clean(b.phones), emails: clean(b.emails).map(lower), domains: clean(b.domains).map(lower) })
  if (b.kind === 'person' && b.companyId) { p.companyId = b.companyId; p.companyName = S.crm.find(x => x.id === b.companyId)?.name || '' }
  json(res, partyOut(p, true))
}
route('POST', '/api/crm/parties', (req, res) => crmSave(req, res, 0))
route('PUT', '/api/crm/parties/{id}', (req, res, q, p) => crmSave(req, res, Number(p.id)))
route('DELETE', '/api/crm/parties/{id}', (req, res, q, p) => { const id = Number(p.id); S.crm = S.crm.filter(x => x.id !== id); for (const x of S.crm) if (x.companyId === id) { x.companyId = null; x.companyName = '' } json(res, { ok: true }) })
route('POST', '/api/crm/parties/{id}/self', (req, res, q, p) => { const x = S.crm.find(y => y.id === Number(p.id)); if (!x) return err(res, 404, 'Card not found'); x.own = true; json(res, { ok: true }) })
route('POST', '/api/crm/parties/{id}/merge', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const from = S.crm.find(y => y.id === Number(p.id)), into = S.crm.find(y => y.id === Number(b.into))
  if (!from || !into) return err(res, 404, 'Card not found')
  for (const k of ['phones', 'emails', 'domains']) into[k] = [...new Set([...into[k], ...from[k]])]
  for (const k of ['inn', 'kpp', 'ogrn', 'legalAddress', 'bankName', 'bik', 'account', 'corrAccount', 'website', 'position', 'notes']) if (!into[k]) into[k] = from[k]
  into.addresses.push(...from.addresses)
  for (const x of S.crm) if (x.companyId === from.id) { x.companyId = into.id; x.companyName = into.name }
  S.crm = S.crm.filter(y => y !== from)
  json(res, partyOut(into, true))
})
route('POST', '/api/crm/parties/{id}/addresses', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const x = S.crm.find(y => y.id === Number(p.id))
  if (!x) return err(res, 404, 'Card not found')
  if (String(b.address || '').trim().length < 5) return err(res, 400, 'Enter an address')
  x.addresses.push({ id: ++S.crmNext.addr, kind: 'delivery', address: String(b.address).trim() })
  json(res, { ok: true })
})
route('DELETE', '/api/crm/addresses/{id}', (req, res, q, p) => { for (const x of S.crm) x.addresses = x.addresses.filter(a => a.id !== Number(p.id)); json(res, { ok: true }) })
route('POST', '/api/crm/suggestions/{id}', (req, res, q, p) => {
  const id = Number(p.id)
  const x = S.crm.find(y => y.suggestions.some(g => g.id === id))
  if (!x) return err(res, 404, 'Suggestion not found')
  const g = x.suggestions.find(z => z.id === id)
  x.suggestions = x.suggestions.filter(z => z.id !== id)
  if (q.accept === '1') {
    const map = { legal_address: 'legalAddress', bank_name: 'bankName', corr_account: 'corrAccount' }
    x[map[g.field] || g.field] = g.value
  }
  json(res, { ok: true, open: x.id })
})
route('POST', '/api/crm/extract', async (req, res) => {
  const b = await jsonBody(req)
  const m = find(lower(b.account), b.folder, b.uid)
  if (!m) return err(res, 404, 'Message not found')
  await sleep(2500)
  const ourAddr = e => !!accountOf(e) || e === S.W.gmail
  const party = ourAddr(m.from.email) ? m.to.find(a => !ourAddr(a.email)) : m.from
  if (!party) return err(res, 422, 'The message has no counterparty details (looks like a newsletter or a notification)')
  const exists = S.crm.find(x => x.kind === 'person' && x.emails.includes(party.email))
  if (exists) return json(res, { id: exists.companyId || exists.id })
  const dom = party.email.split('@')[1]
  const p = { id: ++S.crmNext.party, kind: 'person', name: party.name || party.email, country: '', ids: {}, inn: '', kpp: '', ogrn: '', legalAddress: '', bankName: '', bik: '', account: '', corrAccount: '',
    website: PUBLIC_DOMAINS.has(dom) ? '' : dom, position: '', notes: S.W.L(`Card made by Loggi from the letter "${m.subject}".`, `Карточка создана Loggi по письму «${m.subject}».`), phones: [], emails: [party.email], domains: [], own: false, companyId: null, companyName: '', addresses: [], suggestions: [] }
  const tel = /(\+\d[\d\s()-]{8,}\d)/.exec(m.text)
  if (tel) p.phones.push(tel[1])
  S.crm.push(p)
  json(res, { id: p.id })
})

// --- AI ---
route('GET', '/api/ai/actions', (req, res) => json(res, S.X.prompts.filter(p => p.enabled).map(p => ({ key: p.key, title: p.title, description: p.description }))))
route('POST', '/api/ai/run', async (req, res) => {
  const b = await jsonBody(req)
  const p = S.X.prompts.find(x => x.key === b.key)
  if (!p || !p.enabled) return err(res, 404, 'This AI action has been disabled by the administrator')
  const t0 = Date.now()
  await sleep(b.key === 'reply' || b.key === 'write' ? 1700 : b.key === 'reply_questions' ? 1400 : 1000)
  json(res, { text: aiAnswer(LANG, b.key, b, S.W.U.anna.name), ms: Date.now() - t0 })
})
route('POST', '/api/ai/transcribe', async (req, res) => {
  await rawBody(req)
  await sleep(900)
  json(res, { text: S.W.L('Tell her we can deliver the first ten pumps in four weeks and give four percent discount for the whole batch', 'Напиши, что первые десять насосов поставим за четыре недели и дадим скидку четыре процента на всю партию') })
})
route('GET', '/api/mcp/tokens', (req, res) => json(res, { tokens: S.X.mcpTokens.filter(t => t.login === 'anna').map(t => { const o = { ...t }; delete o.login; return o }), endpoint: `https://${S.W.org.host}/mcp` }))
route('POST', '/api/mcp/tokens', async (req, res) => {
  const b = await jsonBody(req)
  if (!String(b.name || '').trim()) return err(res, 400, 'Name the access, for example "Claude on my laptop"')
  const token = 'tm_' + crypto.randomBytes(24).toString('base64url')
  const t = { id: Math.max(0, ...S.X.mcpTokens.map(x => x.id)) + 1, name: b.name.trim(), prefix: token.slice(0, 12), canSend: !!b.canSend, canModify: !!b.canModify, mailboxes: b.mailboxes || [],
    created: iso(new Date()), lastUsed: null, expires: b.days > 0 ? iso(new Date(Date.now() + b.days * 86400000)) : null, login: 'anna' }
  S.X.mcpTokens.unshift(t)
  json(res, { id: t.id, token, endpoint: `https://${S.W.org.host}/mcp` })
})
route('DELETE', '/api/mcp/tokens/{id}', (req, res, q, p) => { S.X.mcpTokens = S.X.mcpTokens.filter(t => t.id !== Number(p.id)); json(res, { ok: true }) })

// --- live events ---
route('GET', '/api/events', (req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store', 'X-Accel-Buffering': 'no', Connection: 'keep-alive' })
  res.write('retry: 3000\n\n')
  res.write(`data: ${JSON.stringify({ type: 'ready' })}\n\n`)
  clients.add(res)
  const t = setTimeout(() => { const e = dueReminder(); if (e) res.write(`data: ${JSON.stringify({ type: 'reminder', event: evOut(e) })}\n\n`) }, 3000)
  req.on('close', () => { clearTimeout(t); clients.delete(res) })
})

// ---------- administration ----------
const userOut = u => {
  const grants = S.W.mailboxes.flatMap(m => m.users.filter(g => g[0] === u.id).map(g => ({ mailboxId: m.id, address: m.address, canSend: g[1], personal: g[2] })))
    .sort((a, b) => (b.personal - a.personal) || a.address.localeCompare(b.address))
  const seen = { 1: 2, 2: 95, 3: 41, 4: 60 * 75, 5: 60 * 30, 6: 60 * 24 * 12 }[u.id]
  return { id: u.id, login: u.login, name: u.name, isAdmin: !!u.isAdmin, active: u.active !== false, createdAt: iso(new Date(S.now.getTime() - (700 - u.id * 60) * 86400000)),
    lastSeen: seen ? iso(new Date(S.now.getTime() - seen * 60000)) : null, sessions: u.active === false ? 0 : u.id === 1 ? 3 : 1, mailboxes: grants, lang: u.lang || '', timezone: u.timezone || '' }
}
const mbOut = m => ({ id: m.id, address: m.address, domain: m.address.split('@')[1], name: m.name, quotaBytes: m.quotaGB * 2 ** 30, usedBytes: m.deletedAt ? null : Math.round(m.usedMB * 2 ** 20),
  messages: m.deletedAt ? null : m.messages, active: m.active !== false, createdAt: iso(new Date(S.now.getTime() - (600 - m.id * 40) * 86400000)), signature: S.W.signatures[m.address] || '',
  external: !!m.external, deletedAt: m.deletedAt || null, purgeAfter: m.purgeAfter || null, deletedBy: m.deletedBy || '', template: S.W.startTemplates[m.address] || '',
  users: m.users.map(g => { const u = S.W.users.find(x => x.id === g[0]); return { id: g[0], login: u?.login || '?', canSend: g[1], personal: g[2] } }).sort((a, b) => a.login.localeCompare(b.login)) })
const allMailboxes = () => {
  const legacy = { id: 11, address: 'office@nordwave-legacy.example', name: S.W.L('Nordwave (old hosting)', 'Нордвейв (старый хостинг)'), quotaGB: 0, usedMB: 4110, messages: 21877, users: [[1, false, false]], external: true }
  return [...S.W.mailboxes, ...(S.W.mailboxes.some(m => m.id === 11) ? [] : [legacy])]
}
route('GET', '/api/admin/overview', async (req, res) => {
  await sleep(400)
  const X = S.X
  json(res, {
    counts: { users: S.W.users.filter(u => u.active !== false).length, mailboxes: S.W.mailboxes.filter(m => m.active !== false && !m.deletedAt).length, aliases: X.aliases.filter(a => a.active).length,
      domains: X.domains.length, sent24h: 37 + X.sentLog.length, failedLogins24h: 3 },
    domains: X.domains.map(d => ({ id: d.id, name: d.name, checks: X.dns[d.id] || [] })), services: X.services, server: { ...X.server, cpu: { ...X.server.cpu, percent: 5 + Math.floor(Math.random() * 8) } },
    recent: X.audit.slice(0, 10),
  })
})
route('GET', '/api/admin/server', async (req, res) => { await sleep(300); json(res, { ...S.X.server, cpu: { ...S.X.server.cpu, percent: 5 + Math.floor(Math.random() * 8) }, services: S.X.services }) })
route('GET', '/api/admin/audit', (req, res, q) => {
  const limit = Math.min(500, Number(q.limit) || 100)
  const before = Number(q.before) || 0
  if (q.kind === 'sent') return json(res, S.X.sentLog.filter(x => !before || x.id < before).slice(0, limit))
  const list = S.X.audit.filter(a => (!before || a.id < before) && (q.kind === 'login' ? a.action.startsWith('login.') : q.kind === 'admin' ? !a.action.startsWith('login.') : true))
  json(res, list.slice(0, limit))
})
route('GET', '/api/admin/users', (req, res) => json(res, S.W.users.slice().sort((a, b) => a.login.localeCompare(b.login)).map(userOut)))
route('POST', '/api/admin/users', async (req, res) => {
  const b = await jsonBody(req)
  const login = lower(b.login).trim()
  if (!/^[a-z0-9][a-z0-9._-]{1,}$/.test(login)) return err(res, 400, 'Username: Latin letters, digits, dot, hyphen or underscore, at least 2 characters')
  if (S.W.users.some(u => u.login === login)) return err(res, 409, `Username "${login}" is already taken`)
  if (b.password && b.password.length < 10) return err(res, 400, 'Password is too short: at least 10 characters are required')
  const u = { id: Math.max(...S.W.users.map(x => x.id)) + 1, login, name: String(b.name || '').trim(), isAdmin: !!b.isAdmin, personal: '', title: '' }
  S.W.users.push(u)
  for (const g of b.mailboxes || []) { const m = S.W.mailboxes.find(x => x.id === g.mailboxId); if (m) m.users.push([u.id, !!(g.canSend || g.personal), !!g.personal]) }
  S.X.audit.unshift({ id: S.X.audit[0].id + 1, at: iso(new Date()), actor: 'anna', action: 'user.create', target: login, details: { admin: !!b.isAdmin }, ip: '198.51.100.24' })
  json(res, { id: u.id, ...(b.password ? {} : { password: genPass() }) }, 201)
})
const genPass = () => Array.from(crypto.randomBytes(16), x => 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'[x % 56]).join('')
route('PATCH', '/api/admin/users/{id}', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const u = S.W.users.find(x => x.id === Number(p.id))
  if (!u) return err(res, 404, 'User not found')
  if (u.id === 1 && (b.isAdmin === false || b.active === false)) return err(res, 400, 'You cannot disable yourself or remove your own administrator rights')
  for (const k of ['name', 'isAdmin', 'active', 'lang', 'timezone']) if (b[k] !== undefined && b[k] !== null) u[k] = b[k]
  json(res, { ok: true })
})
route('DELETE', '/api/admin/users/{id}', (req, res, q, p) => {
  const id = Number(p.id)
  if (id === 1) return err(res, 400, 'You cannot delete yourself')
  S.W.users = S.W.users.filter(u => u.id !== id)
  for (const m of S.W.mailboxes) m.users = m.users.filter(g => g[0] !== id)
  json(res, { ok: true })
})
route('POST', '/api/admin/users/{id}/password', async (req, res) => { const b = await jsonBody(req); if (b.password && b.password.length < 10) return err(res, 400, 'Password is too short: at least 10 characters are required'); json(res, { ok: true, ...(b.password ? {} : { password: genPass() }) }) })
route('POST', '/api/admin/users/{id}/logout', (req, res) => json(res, { closed: 2 }))
route('PUT', '/api/admin/users/{id}/mailboxes', async (req, res, q, p) => {
  const id = Number(p.id)
  const grants = await jsonBody(req)
  for (const m of S.W.mailboxes) m.users = m.users.filter(g => g[0] !== id)
  for (const g of Array.isArray(grants) ? grants : []) { const m = S.W.mailboxes.find(x => x.id === g.mailboxId); if (m) m.users.push([id, !!(g.canSend || g.personal), !!g.personal]) }
  json(res, { ok: true })
})
route('GET', '/api/admin/mailboxes', (req, res) => json(res, allMailboxes().slice().sort((a, b) => a.address.split('@')[1].localeCompare(b.address.split('@')[1]) || a.address.localeCompare(b.address)).map(mbOut)))
route('POST', '/api/admin/mailboxes', async (req, res) => {
  const b = await jsonBody(req)
  const address = `${lower(b.localPart).trim()}@${lower(b.domain)}`
  if (S.W.mailboxes.some(m => m.address === address)) return err(res, 409, `Mailbox ${address} already exists`)
  const m = { id: Math.max(...allMailboxes().map(x => x.id)) + 1, address, name: String(b.name || '').trim(), quotaGB: (Number(b.quotaBytes) || 0) / 2 ** 30, usedMB: 0, messages: 0, users: (b.grantUsers || []).map(id => [id, true, false]) }
  S.W.mailboxes.push(m)
  json(res, { id: m.id, address, ...(b.password ? {} : { password: genPass() }) }, 201)
})
route('PATCH', '/api/admin/mailboxes/{id}', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const m = allMailboxes().find(x => x.id === Number(p.id))
  if (!m) return err(res, 404, 'Mailbox not found')
  if (b.name != null) m.name = b.name
  if (b.quotaBytes != null) m.quotaGB = b.quotaBytes / 2 ** 30
  if (b.active != null) m.active = b.active
  if (b.signature != null) S.W.signatures[m.address] = b.signature
  if (b.template != null) S.W.startTemplates[m.address] = b.template
  json(res, { ok: true })
})
route('PUT', '/api/admin/mailboxes/{id}/users', async (req, res, q, p) => {
  const m = allMailboxes().find(x => x.id === Number(p.id))
  if (!m) return err(res, 404, 'Mailbox not found')
  const list = await jsonBody(req)
  m.users = (Array.isArray(list) ? list : []).map(g => [g.userId, !!(g.canSend || g.personal), !!g.personal])
  json(res, { ok: true })
})
route('POST', '/api/admin/mailboxes/{id}/password', async (req, res) => { const b = await jsonBody(req); json(res, { ok: true, ...(b.password ? {} : { password: genPass() }) }) })
route('POST', '/api/admin/mailboxes/{id}/delete', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const m = allMailboxes().find(x => x.id === Number(p.id))
  if (!m) return err(res, 404, 'Mailbox not found')
  if (lower(b.confirm).trim() !== m.address) return err(res, 400, 'Type the mailbox address to confirm')
  if (b.now) { S.W.mailboxes = S.W.mailboxes.filter(x => x !== m); return json(res, { ok: true }) }
  m.deletedAt = iso(new Date()); m.purgeAfter = iso(new Date(Date.now() + 30 * 86400000)); m.deletedBy = 'anna'; m.active = false
  json(res, { ok: true })
})
route('POST', '/api/admin/mailboxes/{id}/restore', (req, res, q, p) => {
  const m = allMailboxes().find(x => x.id === Number(p.id))
  if (!m) return err(res, 404, 'Mailbox not found')
  delete m.deletedAt; delete m.purgeAfter; delete m.deletedBy; m.active = true
  json(res, { ok: true })
})
route('GET', '/api/admin/aliases', (req, res) => json(res, S.X.aliases))
route('POST', '/api/admin/aliases', async (req, res) => {
  const b = await jsonBody(req)
  const source = lower(b.source)
  if (!/^[^@\s]+@[a-z0-9.-]+$/.test(source)) return err(res, 400, 'Invalid alias address')
  if (S.X.aliases.some(a => a.source === source) || S.W.mailboxes.some(m => m.address === source)) return err(res, 409, `The address ${source} is already in use`)
  const a = { id: Math.max(0, ...S.X.aliases.map(x => x.id)) + 1, source, destinations: b.destinations || [], active: true, domain: source.split('@')[1], createdAt: iso(new Date()) }
  S.X.aliases.push(a)
  json(res, { id: a.id }, 201)
})
route('PATCH', '/api/admin/aliases/{id}', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const a = S.X.aliases.find(x => x.id === Number(p.id))
  if (!a) return err(res, 404, 'Alias not found')
  if (b.destinations) a.destinations = b.destinations
  if (b.active != null) a.active = b.active
  json(res, { ok: true })
})
route('DELETE', '/api/admin/aliases/{id}', (req, res, q, p) => { S.X.aliases = S.X.aliases.filter(x => x.id !== Number(p.id)); json(res, { ok: true }) })
route('GET', '/api/admin/domains', (req, res) => json(res, S.X.domains.map(d => ({ ...d, mailboxes: S.W.mailboxes.filter(m => m.address.endsWith('@' + d.name)).length, aliases: S.X.aliases.filter(a => a.domain === d.name).length }))))
route('POST', '/api/admin/domains', async (req, res) => {
  const b = await jsonBody(req)
  const name = lower(b.name).trim()
  if (!/^([a-z0-9-]+\.)+[a-z]{2,}$/.test(name)) return err(res, 400, 'Invalid domain name')
  if (S.X.domains.some(d => d.name === name)) return err(res, 409, `Domain ${name} already exists`)
  const d = { id: Math.max(0, ...S.X.domains.map(x => x.id)) + 1, name, active: true, createdAt: iso(new Date()), dkimSelector: 'tm' + new Date().getFullYear() }
  S.X.domains.push(d)
  S.X.dns[d.id] = S.X.dns[2].map(c => ({ ...c, status: c.key === 'a' || c.key === 'ptr' ? 'ok' : 'missing', found: c.key === 'a' || c.key === 'ptr' ? c.found : '', name: c.key === 'dkim' ? d.dkimSelector + '._domainkey' : c.name,
    expected: c.expected.replace(/nordwave-parts\.example/g, name), hint: c.key === 'mx' ? 'Without an MX record, mail for this domain is not delivered.' : c.key === 'spf' ? 'Without SPF, messages end up in spam more often.' : c.key === 'dkim' ? 'Without DKIM, recipients cannot verify that a message really comes from you.' : c.key === 'dmarc' ? 'Gmail and other large providers trust domains without DMARC less.' : '' }))
  json(res, { id: d.id }, 201)
})
route('DELETE', '/api/admin/domains/{id}', (req, res, q, p) => {
  const d = S.X.domains.find(x => x.id === Number(p.id))
  if (!d) return err(res, 404, 'Domain not found')
  if (S.W.mailboxes.some(m => m.address.endsWith('@' + d.name))) return err(res, 400, 'The domain still has mailboxes. Delete them first')
  S.X.domains = S.X.domains.filter(x => x !== d)
  json(res, { ok: true })
})
route('GET', '/api/admin/domains/{id}/dns', async (req, res, q, p) => {
  const d = S.X.domains.find(x => x.id === Number(p.id))
  if (!d) return err(res, 404, 'Domain not found')
  await sleep(600)
  json(res, { domain: d.name, checks: S.X.dns[d.id] || [], checkedAt: iso(new Date()) })
})
route('GET', '/api/admin/ai', async (req, res) => { await sleep(300); json(res, { status: S.X.aiStatus, prompts: S.X.prompts }) })
route('GET', '/api/admin/ai/provider', (req, res) => json(res, S.X.aiProvider))
route('PUT', '/api/admin/ai/provider', async (req, res) => {
  const b = await jsonBody(req)
  if (!b.url || !String(b.model || '').trim()) return err(res, 400, 'Specify the API URL and the model')
  Object.assign(S.X.aiProvider.provider, { kind: b.kind, name: b.name || 'Loggi', url: b.url, model: b.model, asrUrl: b.asrUrl || '', asrModel: b.asrModel || '' })
  Object.assign(S.X.aiStatus, { kind: b.kind, name: b.name || 'Loggi', model: b.model, host: (() => { try { return new URL(b.url).host } catch { return '' } })() })
  json(res, { ok: true })
})
route('PUT', '/api/admin/ai/prompts/{key}', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const x = S.X.prompts.find(y => y.key === p.key)
  if (!x) return err(res, 404, 'Prompt not found')
  if (!String(b.title || '').trim() || !String(b.template || '').trim()) return err(res, 400, 'A title and a request template are required')
  Object.assign(x, { title: b.title, description: b.description, system: b.system, template: b.template, model: b.model, temperature: b.temperature, maxTokens: b.maxTokens, enabled: !!b.enabled, updatedAt: iso(new Date()), updatedBy: 'anna' })
  json(res, { ok: true })
})
route('POST', '/api/admin/ai/test', async (req, res) => {
  const b = await jsonBody(req)
  const t0 = Date.now()
  await sleep(1500)
  const inp = b.input || {}
  const request = String(b.prompt?.template || '').replace(/\{(\w+)\}/g, (m, k) => inp[k] || (k === 'user' ? S.W.U.anna.name : '—'))
  json(res, { text: aiAnswer(LANG, b.prompt?.key || 'reply', inp, S.W.U.anna.name), request, ms: Date.now() - t0 })
})
route('GET', '/api/admin/crm', (req, res) => json(res, { companies: S.crm.filter(x => x.kind === 'company').length, people: S.crm.filter(x => x.kind === 'person').length, files: 214, analysed: 3871,
  job: { running: false, done: 0, total: 0, found: 0, errors: 0, started: '0001-01-01T00:00:00Z', current: '' } }))
route('POST', '/api/admin/crm/backfill', (req, res) => json(res, { ok: true }))
route('POST', '/api/admin/crm/reprocess', (req, res) => json(res, { ok: true }))
route('GET', '/api/admin/import-mbox', (req, res) => json(res, { running: false }))
route('GET', '/api/admin/collector', (req, res) => json(res, S.X.collector))
route('POST', '/api/admin/collector', async (req, res) => {
  const b = await jsonBody(req)
  if (!/^[^@\s]+@[^@\s]+\.[a-z]{2,}$/i.test(b.address || '')) return err(res, 400, 'Specify the external mailbox address')
  if (!b.host || !b.port || !b.username) return err(res, 400, 'Specify the server, port and username')
  const a = { ...b, id: Math.max(0, ...S.X.collector.map(x => x.id)) + 1, address: lower(b.address), mailbox: lower(b.address), status: 'new', lastError: '', imported: 0, hasSmtpPassword: !!b.smtpPassword, remaining: 0, remainingBytes: 0 }
  delete a.password; delete a.smtpPassword
  S.X.collector.push(a)
  json(res, { id: a.id })
})
route('PUT', '/api/admin/collector/{id}', async (req, res, q, p) => {
  const b = await jsonBody(req)
  const a = S.X.collector.find(x => x.id === Number(p.id))
  if (!a) return err(res, 404, 'Not found')
  const keep = { id: a.id, address: a.address, mailbox: a.mailbox, status: a.status, lastError: a.lastError, lastRun: a.lastRun, imported: a.imported, remaining: a.remaining, remainingBytes: a.remainingBytes }
  Object.assign(a, b, keep, { hasSmtpPassword: a.hasSmtpPassword || !!b.smtpPassword })
  delete a.password; delete a.smtpPassword
  json(res, { ok: true })
})
route('DELETE', '/api/admin/collector/{id}', (req, res, q, p) => { S.X.collector = S.X.collector.filter(x => x.id !== Number(p.id)); json(res, { ok: true }) })
route('POST', '/api/admin/collector/test', async (req, res) => {
  const b = await jsonBody(req)
  await sleep(1800)
  const out = { mail: /gmail/.test(b.host || '') || /legacy/.test(b.host || '') ? 'Connected, 9381 messages in Inbox' : 'Connected, 1204 messages in Inbox' }
  if (b.smtpHost) out.smtp = 'Sending: signed in'
  json(res, out)
})
route('POST', '/api/admin/collector/{id}/sync', (req, res, q, p) => {
  const a = S.X.collector.find(x => x.id === Number(p.id))
  if (!a) return err(res, 404, 'Not found')
  a.progress = S.W.L('INBOX: 12 of 40', 'INBOX: 12 из 40')
  setTimeout(() => { delete a.progress; a.lastRun = iso(new Date()); a.imported += a.status === 'error' ? 0 : 3; if (a.status === 'new') a.status = 'ok' }, 4000)
  json(res, { ok: true })
})
route('GET', '/api/admin/sender-rules', (req, res) => json(res, S.X.senderRules))
route('DELETE', '/api/admin/sender-rules/{id}', (req, res, q, p) => { S.X.senderRules = S.X.senderRules.filter(x => x.id !== Number(p.id)); json(res, { ok: true }) })
route('GET', '/api/admin/antivirus', async (req, res) => { await sleep(300); json(res, S.X.antivirus) })
route('POST', '/api/admin/antivirus/test', async (req, res) => { await sleep(500); json(res, { detected: true, virus: 'Eicar-Signature', ms: 38 }) })
route('POST', '/api/admin/antivirus/quarantine/{id}', (req, res, q, p) => {
  const item = S.X.antivirus.quarantine.find(x => x.id === Number(p.id))
  if (!item) return err(res, 404, 'Message not found in quarantine')
  if (q.action === 'delete') S.X.antivirus.quarantine = S.X.antivirus.quarantine.filter(x => x !== item)
  else if (q.action === 'release') { item.released = iso(new Date()); item.releasedBy = 'anna' }
  else return err(res, 400, 'Unknown action')
  json(res, { ok: true })
})
route('GET', '/api/admin/mcp-tokens', (req, res) => json(res, { tokens: S.X.mcpTokens }))
route('DELETE', '/api/admin/mcp-tokens/{id}', (req, res, q, p) => { S.X.mcpTokens = S.X.mcpTokens.filter(t => t.id !== Number(p.id)); json(res, { ok: true }) })

// ---------- non-API addresses: file links, thumbnails, signature pictures, "remote" pictures ----------
function filePage(req, res, f) {
  const ru = LANG === 'ru'
  const T = (en, r) => (ru ? r : en)
  const ext = (f.name.split('.').pop() || '').toUpperCase()
  const exp = new Date(f.expires).toISOString().slice(0, 10)
  const pv = f.token === 'kp300-drawings' || f.thumb ? `<img src="/t/${esc(f.token)}.jpg" alt="">` : `<div class="ext">${esc(ext)}</div>`
  send(res, 200, `<!doctype html><html lang="${LANG}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(f.name)} — ${esc(S.W.org.name)}</title>
<style>*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#eef2f6;font:15px/1.45 Arial,Helvetica,sans-serif;color:#1d2733;padding:24px}
.card{background:#fff;border-radius:9px;box-shadow:0 10px 40px rgba(20,40,70,.12);width:min(520px,100%);overflow:hidden}.brand{display:flex;align-items:center;gap:10px;padding:16px 22px;border-bottom:1px solid #e6ebf0;font-weight:700;letter-spacing:.04em;color:#0a5fd7}
.brand i{width:10px;height:10px;border-radius:1.5px;background:linear-gradient(135deg,#0a5fd7,#20c5f5)}.pv{background:#f4f6f9;display:grid;place-items:center;min-height:180px;border-bottom:1px solid #e6ebf0;padding:18px}
.pv img{display:block;max-width:100%;max-height:360px;border-radius:3px;box-shadow:0 2px 12px rgba(20,40,70,.18);background:#fff}.ext{width:84px;height:104px;border-radius:5px;background:#fff;border:1px solid #dfe5ec;display:grid;place-items:center;font-weight:700;color:#0a5fd7;font-size:18px}
.body{padding:20px 22px 24px}h1{font-size:18px;margin:0 0 6px;word-break:break-word}.meta{color:#5f6b78;font-size:13.5px;margin:0 0 18px}.btn{display:block;text-align:center;background:#0a5fd7;color:#fff;text-decoration:none;font-weight:700;padding:13px 16px;border-radius:6px}
.av{font-size:13px;margin:12px 0 0;text-align:center;color:#1e8048}.foot{color:#8a96a3;font-size:12px;margin-top:14px;text-align:center}</style></head><body><div class="card"><div class="brand"><i></i>${esc(S.W.org.name)}</div>
<div class="pv">${pv}</div><div class="body"><h1>${esc(f.name)}</h1><p class="meta">${humanSize(f.size, ru)}${f.sender ? ' · ' + T('sent by ', 'отправил ') + esc(f.sender) : ''}<br>${T('The link is valid until ', 'Ссылка действует до ')}${exp}</p>
<a class="btn" href="/f/${esc(f.token)}/download">${T('Download', 'Скачать')}</a><p class="av">✓ ${T('Checked by antivirus', 'Проверено антивирусом')}</p><p class="foot">${T(`File shared via ${esc(S.W.org.name)} mail`, `Файл передан через почту ${esc(S.W.org.name)}`)}</p></div></div></body></html>`,
  { 'Content-Type': 'text/html; charset=utf-8' })
}
function fileDownload(req, res, f) {
  let data = f.data
  if (!data) data = f.mime === 'application/pdf' ? asset(`${LANG}/spec.pdf`) : Buffer.from(`ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('Nordwave demo model: KP-300/45 assembly'),'2;1');\nFILE_NAME('${f.name}','${iso(S.now)}',('Nordwave'),('Nordwave Engineering'),'','','');\nFILE_SCHEMA(('AUTOMOTIVE_DESIGN'));\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;\n`)
  send(res, 200, data, { 'Content-Type': 'application/octet-stream', 'Content-Disposition': disposition('attachment', f.name) })
}
function extra(req, res, p) {
  let m
  if ((m = /^\/f\/([^/]+)(\/download)?$/.exec(p))) {
    const f = S.bigfiles[decodeURIComponent(m[1])]
    if (!f) return send(res, 404, `<!doctype html><meta charset="utf-8"><title>File unavailable</title><p style="font:16px Arial;padding:40px">${LANG === 'ru' ? 'Файл больше недоступен' : 'This file is no longer available'}</p>`, { 'Content-Type': 'text/html; charset=utf-8' })
    return m[2] ? fileDownload(req, res, f) : filePage(req, res, f)
  }
  if ((m = /^\/t\/([^/]+)\.jpg$/.exec(p))) {
    const f = S.bigfiles[decodeURIComponent(m[1])]
    if (f?.data && /^image\//.test(f.mime)) return send(res, 200, f.data, { 'Content-Type': f.mime })
    if (f) return send(res, 200, asset('common/drawing.jpg'), { 'Content-Type': 'image/jpeg', 'Cache-Control': 'public, max-age=86400' })
    return send(res, 404, '')
  }
  if ((m = /^\/i\/([^/]+)$/.exec(p))) {
    if (m[1] === 'nordwave-logo.png') return send(res, 200, asset('common/logo.png'), { 'Content-Type': 'image/png', 'Cache-Control': 'public, max-age=86400' })
    const img = S.sigImages[m[1]]
    return img ? send(res, 200, img.data, { 'Content-Type': img.type }) : send(res, 404, '')
  }
  if ((m = /^\/demo-remote\/(news-banner|news-product)\.png$/.exec(p))) return send(res, 200, asset(`${LANG}/${m[1]}.png`), { 'Content-Type': 'image/png', 'Access-Control-Allow-Origin': '*' })
  return false
}

// ---------- demo controls ----------
async function demo(req, res, p, q) {
  if (p === '/demo/push') {
    const n = Math.max(1, Math.min(10, Number(q.n) || 1))
    const out = []
    for (let i = 0; i < n; i++) {
      let spec
      if (q.id) spec = S.queue.find(x => x.id === q.id)
      else if (q.kind === 'filed') spec = S.queue.find(x => x.filed)
      else spec = S.queue[S.pushed % S.queue.length]
      if (!spec) return err(res, 404, 'No such letter in the queue (n1…n4)')
      const m = pushIncoming(spec)
      out.push({ account: m.account, folder: m.folder, uid: m.uid, subject: m.subject })
      if (n > 1) await sleep(400)
    }
    return json(res, { ok: true, pushed: out, clients: clients.size })
  }
  if (p === '/demo/reminder') {
    const e = S.X.due
    S.reminderDone.delete(e.id); delete S.snoozed[e.id]
    publish({ type: 'reminder', event: evOut(e) })
    return json(res, { ok: true, clients: clients.size })
  }
  if (p === '/demo/reminder-done') { S.reminderDone.add(S.X.due.id); return json(res, { ok: true }) }
  if (p === '/demo/reset') { build(); DICT = null; publish({ type: 'ready' }); return json(res, { ok: true }) }
  return err(res, 404, 'Unknown demo command')
}

// ---------- static files (the SPA) ----------
function serveStatic(req, res, p) {
  let rel = path.normalize(decodeURIComponent(p)).replace(/^(\.\.[/\\])+/, '')
  let file = path.join(PUBLIC, rel)
  if (!file.startsWith(PUBLIC)) return send(res, 403, '')
  let st = null
  try { st = fs.statSync(file) } catch {}
  if (st?.isDirectory()) { file = path.join(file, 'index.html'); try { st = fs.statSync(file) } catch { st = null } }
  if (!st || !st.isFile()) { file = path.join(PUBLIC, 'index.html'); st = fs.statSync(file) }
  const type = MIME[path.extname(file)] || 'application/octet-stream'
  const cache = p.startsWith('/_nuxt/') && !p.includes('/builds/') ? 'public, max-age=31536000, immutable' : 'no-cache'
  res.writeHead(200, { 'Content-Type': type, 'Content-Length': st.size, 'Cache-Control': cache })
  if (req.method === 'HEAD') return res.end()
  fs.createReadStream(file).pipe(res)
}

// ---------- the server ----------
const server = http.createServer(async (req, res) => {
  const u = new URL(req.url, 'http://x')
  const p = u.pathname
  const q = Object.fromEntries(u.searchParams)
  try {
    if (p.startsWith('/demo/')) return await demo(req, res, p, q)
    if (p.startsWith('/api/')) {
      for (const r of routes) {
        if (r.method !== req.method) continue
        const m = r.re.exec(p)
        if (!m) continue
        if (!r.open && !signedIn(req)) return err(res, 401, 'Please sign in')
        const params = Object.fromEntries(r.keys.map((k, i) => [k, decodeURIComponent(m[i + 1])]))
        return await r.fn(req, res, q, params)
      }
      if (routes.some(r => r.re.test(p))) return err(res, 405, 'Method not allowed')
      return err(res, 404, 'No such API method')
    }
    if (extra(req, res, p) !== false) return
    if (req.method !== 'GET' && req.method !== 'HEAD') return send(res, 405, '')
    serveStatic(req, res, p)
  } catch (e) {
    console.error(req.method, p, e)
    if (!res.headersSent) err(res, 500, 'Demo server error: ' + e.message)
    else res.end()
  }
})

build()
server.listen(PORT, BIND, () => {
  console.log(`TMAIL demo (${LANG}) on http://${BIND === '0.0.0.0' ? 'localhost' : BIND}:${PORT}/  —  ${S.msgs.length} letters, ${S.events.length} events, ${S.crm.length} cards${AUTO_LOGIN ? ', auto-login' : ', sign in with any name and password'}`)
  console.log('New letter: curl -X POST http://localhost:' + PORT + '/demo/push   ·   reminder: /demo/reminder   ·   reset: /demo/reset')
})
