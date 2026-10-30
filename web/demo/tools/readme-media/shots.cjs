// README screenshots from the demo server: node shots.cjs <port> <en|ru> [only-prefix…]
const { W, launch, setup, HOST, fs } = require('./lib.cjs')
const PORT = process.argv[2], LANG = process.argv[3], ONLY = process.argv.slice(4)
const OUT = `/work/work_mail/docs/screenshots/${LANG}`
const L = (en, ru) => (LANG === 'ru' ? ru : en)
const D = 'nordwave.example'
const errs = []
let p, b

const quiet = async (ms = 500) => { await p.waitForFunction(() => !document.querySelector('.veil'), { timeout: 20000 }).catch(() => errs.push('veil stayed')); await W(ms) }
async function save(name) {
  let buf = Buffer.from(await p.screenshot({ type: 'webp', quality: 82 }))
  for (const q of [72, 62, 52]) { if (buf.length <= 250 * 1024) break; buf = Buffer.from(await p.screenshot({ type: 'webp', quality: q })) }
  fs.writeFileSync(`${OUT}/${name}.webp`, buf)
  console.log('saved', name, Math.round(buf.length / 1024) + 'KB')
}
const api = (path, opts = {}) => p.evaluate(async (path, opts) => { const r = await fetch(path, { ...opts, headers: { 'X-ZM': '1', 'Content-Type': 'application/json' } }); const t = await r.text(); try { return JSON.parse(t) } catch { return t } }, path, opts)
const demo = path => p.evaluate(path => fetch(path, { method: 'POST' }).then(r => r.json()), path)
async function go(path = '/') { await p.goto(HOST + path, { waitUntil: 'load' }); await p.waitForSelector('.row, .adm-page, .login, .crm', { timeout: 20000 }).catch(() => {}); await quiet(900) }
async function clickText(sel, text, opts = {}) {
  const ok = await p.$$eval(sel, (els, t, last) => { const xs = els.filter(x => x.textContent.includes(t) && x.offsetParent !== null); const el = last ? xs[xs.length - 1] : xs[0]; if (el) { el.scrollIntoView({ block: 'center' }); el.click(); return true } return false }, text, !!opts.last)
  if (!ok) throw new Error(`no ${sel} with "${text}"`)
}
async function find(acct, folderParam, subj) {
  const r = await api(`/api/messages?account=${acct}&${folderParam}&limit=200`)
  const m = r.messages.find(x => x.subject.includes(subj))
  if (!m) throw new Error('no letter ' + subj)
  return m
}
async function openLetter(acct, role, folder, subj) {
  const m = await find(acct, folder ? 'folder=' + encodeURIComponent(folder) : 'role=' + role, subj)
  const q = new URLSearchParams({ box: acct, ...(folder ? { folder } : role !== 'inbox' ? { f: role } : {}), m: `${acct}/${m.folder}/${m.uid}` })
  await go('/?' + q)
  await p.waitForSelector('.reader .subject', { timeout: 15000 })
  await quiet(1200)
  return m
}
const scrollReader = (y) => p.evaluate(y => { const b = document.querySelector('.reader-body'); if (b) b.scrollTop = y === 'end' ? b.scrollHeight : y }, y)
const scrollTo = sel => p.evaluate(sel => { const el = document.querySelector(sel); if (el) el.scrollIntoView({ block: 'center' }) }, sel)
const tbBtn = title => p.$$eval('.tb-btn', (bs, re) => { const x = bs.find(b => new RegExp(re).test(b.title + ' ' + b.textContent)); if (!x) throw new Error('no toolbar ' + re); x.click() }, title)
async function previewAtt(nameStart) {
  await p.$$eval('.reader .att', (els, t) => { const el = els.find(x => x.textContent.includes(t)); if (!el) throw new Error('no att ' + t); el.click() }, nameStart)
  await W(800); await quiet(1500)
  await p.waitForFunction(() => !document.querySelector('.ocr-img.waiting'), { timeout: 15000 }).catch(() => {}); await W(700)
}
const closePreview = async () => { await p.keyboard.press('Escape'); await W(500) }
async function typeInEditor(sel, text) {
  await p.evaluate(sel => { const el = document.querySelector(sel); el.focus(); const first = el.firstElementChild && !el.firstElementChild.matches('[data-tmail-sig],[data-tmail-quote]') ? el.firstElementChild : null; const r = document.createRange(); if (first) r.selectNodeContents(first); else { r.setStart(el, 0); r.collapse(true) } const s = getSelection(); s.removeAllRanges(); s.addRange(r) }, sel)
  await p.keyboard.type(text, { delay: 8 })
}
// The rectangle of some text inside an element (for hovering / right-clicking it).
const rectOf = (sel, text) => p.evaluate((sel, text) => {
  const root = document.querySelector(sel)
  const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let n = tw.nextNode(); n; n = tw.nextNode()) {
    const i = n.data.indexOf(text)
    if (i >= 0) { const r = document.createRange(); r.setStart(n, i); r.setEnd(n, i + text.length); const b = r.getBoundingClientRect(); return { x: b.left + b.width / 2, y: b.top + b.height / 2 } }
  }
  return null
}, sel, text)

const shots = []
const shot = (name, fn) => shots.push({ name, fn })

// ---------------------------------------------------------------- mail
shot('01-sign-in', async () => {
  await p.deleteCookie({ name: 'zm_sess', domain: 'mail.nordwave.example' })
  await p.goto(HOST + '/login', { waitUntil: 'load' }); await p.waitForSelector('#login'); await W(600)
  await p.type('#login', 'anna'); await p.type('#password', 'correct-horse'); await W(200)
  await save('01-sign-in')
  await p.click('button[type=submit]'); await p.waitForSelector('.row'); await quiet(1500)
})
shot('02-inbox-all-mailboxes', async () => { await go('/'); await save('02-inbox-all-mailboxes') })
shot('03-shared-mailbox', async () => {
  await go('/'); await clickText('.nav-item.acct', 'sales@'); await quiet(1200); await p.evaluate(() => { document.querySelector('.side-scroll').scrollTop = 0 })
  await clickText('.row', L('PO 4471', '4471')); await quiet(1500)
  await save('03-shared-mailbox')
})
shot('04-conversation-counterparty', async () => {
  await openLetter('sales@' + D, 'sent', '', L('Re: Updated price list', 'Re: Обновлённый прайс'))
  await save('04-conversation-counterparty')
})
shot('05-thread', async () => {
  await openLetter('anna@' + D, 'inbox', '', L('Request for quotation', 'Запрос КП'))
  await p.$$eval('.thread-more', b => b[0] && b[0].click()); await W(800)
  await save('05-thread')
})
shot('06-preview-pdf', async () => {
  await go('/?box=sales@' + D)
  await p.$$eval('.file-pill', (els, t) => { const el = els.find(x => x.textContent.includes(t)); el.click() }, L('PO-4471', 'Заказ_4471'))
  await W(1000); await quiet(2500); await save('06-preview-pdf'); await closePreview()
})
shot('07-preview-xlsx', async () => {
  await go('/?box=sales@' + D)
  await p.$$eval('.file-pill', (els, t) => { const el = els.find(x => x.textContent.includes(t)); el.click() }, L('Brightline_PriceList', 'Прайс_'))
  await W(1000); await quiet(2500); await save('07-preview-xlsx'); await closePreview()
})
shot('08-preview-docx', async () => {
  await openLetter('anna@' + D, 'inbox', '', L('Distribution agreement', 'Дилерский договор'))
  await previewAtt(L('Kestrel_Nordwave', 'Договор_')); await W(1500); await save('08-preview-docx'); await closePreview()
})
shot('09-preview-scan-ocr', async () => {
  await openLetter('sales@' + D, 'inbox', '', L('PO 4471', '4471'))
  await previewAtt(L('DN-77812', 'Накладная'))
  await p.waitForSelector('.pv-ocr span', { timeout: 20000 })
  await W(800)
  await p.evaluate(() => {
    const spans = [...document.querySelectorAll('.pv-ocr span')]
    const a = spans.findIndex(s => /Goods|Груз/.test(s.textContent))
    const z = Math.min(spans.length - 1, a + 14)
    const r = document.createRange(); r.setStart(spans[a].firstChild, 0); r.setEnd(spans[z].firstChild, spans[z].firstChild.length)
    getSelection().removeAllRanges(); getSelection().addRange(r)
  })
  await W(300); await save('09-preview-scan-ocr'); await closePreview()
})
shot('10-preview-photo', async () => {
  await openLetter('anna@' + D, '', L('Suppliers', 'Поставщики'), L('Sample castings', 'Пробные отливки'))
  await previewAtt('HV-1187'); await W(1500); await save('10-preview-photo'); await closePreview()
})
shot('11-attachments-antivirus-zip', async () => {
  await openLetter('anna@' + D, '', L('Suppliers', 'Поставщики'), L('Sample castings', 'Пробные отливки'))
  await W(2500); await scrollTo('#main-atts'); await W(600)
  await save('11-attachments-antivirus-zip')
})
shot('12-large-file-link', async () => {
  await openLetter('anna@' + D, 'sent', '', L('drawings and 3D model', 'Чертежи и 3D-модель'))
  await W(1500); await save('12-large-file-link')
})
shot('13-large-file-page', async () => { await p.goto(HOST + '/f/kp300-drawings', { waitUntil: 'load' }); await W(800); await save('13-large-file-page') })
shot('14-phishing-warning', async () => { await openLetter('anna@' + D, 'inbox', '', L('mailbox will be suspended', 'будет заблокирован')); await save('14-phishing-warning') })
shot('15-newsletter-images-blocked', async () => { await openLetter('anna@' + D, 'inbox', '', L('5 trends', '5 трендов')); await W(800); await save('15-newsletter-images-blocked') })

// ---------------------------------------------------------------- compose
shot('16-reply-from-shared-address', async () => {
  await openLetter('sales@' + D, 'inbox', '', L('PO 4471', '4471'))
  await p.$$eval('.reply-bar .btn', b => b[0].click()); await W(1200)
  await typeInEditor('.reader .composer [contenteditable=true], .reader [contenteditable=true]', L('Dear Daniel,\n\nthank you for confirming the delivery. The invoice and the commissioning certificates follow today.', 'Сергей, добрый день!\n\nСпасибо, что подтвердили получение. Счёт-фактуру и паспорта пришлём сегодня.'))
  await W(400); await scrollTo('.from-note'); await p.evaluate(() => document.querySelector('.reader-body').scrollBy(0, 160)); await W(700)
  await save('16-reply-from-shared-address')
  await tbBtn('Templates|Шаблоны'); await W(1800); await save('17-compose-templates'); await p.keyboard.press('Escape'); await W(400)
  await tbBtn('Loggi'); await W(900); await save('18-compose-loggi-menu'); await p.keyboard.press('Escape'); await W(400)
  await p.$$eval('#reader-dock .btn.primary, .composer .btn.primary', b => b[b.length - 1].click()); await W(1600)
  await save('19-send-undo')
  await p.$$eval('.toast button', bs => { const x = bs.find(b => /Undo|Отменить/.test(b.textContent)); x && x.click() }); await W(1200)
})
shot('20-forgotten-attachment', async () => {
  await go('/'); await p.click('.compose-btn'); await W(1000)
  await p.type('#c-to', L('femke', 'фролова')); await W(900); await p.keyboard.press('Enter'); await W(300)
  await p.type('#c-subj', L('Updated price list for the HT-40 tugs', 'Обновлённый прайс для буксиров'))
  await typeInEditor('.cw [contenteditable=true]', L('Dear Femke,\n\nplease find attached our updated price list for the spare parts kits.', 'Елена, добрый день!\n\nВо вложении высылаю обновлённый прайс на комплекты ЗИП.'))
  await W(400)
  await p.$$eval('.cw .btn.primary', b => b[b.length - 1].click()); await W(2000)
  await save('20-forgotten-attachment')
  await p.$$eval('.modal .btn', bs => { const x = bs.find(b => !b.classList.contains('primary')); x && x.click() }).catch(() => p.keyboard.press('Escape')); await W(600)
  await p.$$eval('.cw .icon-btn', bs => bs[bs.length - 1].click()); await W(800)
})
shot('21-spellcheck-tooltip', async () => {
  await go('/'); await p.click('.compose-btn'); await W(1000)
  await p.type('#c-subj', L('Customer portal', 'Портал для клиентов'))
  const text = L('Hello Femke,\n\nour new customer portal will make thisf eature available next week. You will recieve the login by e-mail.', 'Елена, добрый день!\n\nНа следующей неделе этотф ункционал будет доступен в нашем новом портале для клиентов.')
  await typeInEditor('.cw [contenteditable=true]', text); await p.mouse.click(5, 5); await W(3000)
  const word = L('thisf eature', 'этотф ункционал')
  const r = await rectOf('.cw [contenteditable=true]', word)
  await p.mouse.move(r.x - 30, r.y); await p.mouse.move(r.x, r.y, { steps: 6 }); await W(1600)
  await save('21-spellcheck-tooltip')
  await p.mouse.move(r.x, r.y + 150); await W(500)
  await p.mouse.click(r.x, r.y, { button: 'right' }); await W(800)
  await save('22-context-menu')
  await p.keyboard.press('Escape'); await W(300)
  await p.$$eval('.cw .icon-btn', bs => bs[bs.length - 1].click()); await W(800)
})

// ---------------------------------------------------------------- AI
shot('23-ai-summary', async () => {
  await openLetter('anna@' + D, 'inbox', '', L('Request for quotation', 'Запрос КП'))
  await p.$$eval('.reader-bar .icon-btn', bs => { const x = bs.find(b => /Loggi/.test(b.title)); x.click() }); await W(2600)
  await scrollTo('.ai-box'); await p.evaluate(() => document.querySelector('.reader-body').scrollBy(0, 120)); await W(500)
  await save('23-ai-summary')
})
shot('24-ai-reply-questions', async () => {
  await openLetter('anna@' + D, 'inbox', '', L('Request for quotation', 'Запрос КП'))
  await p.$$eval('.reply-bar .btn.ai', bs => bs[0].click()); await W(2600)
  await p.$$eval('.arp-q', qs => { qs.forEach((q, i) => { const c = q.querySelectorAll('.chip'); const k = [0, 2, 0, 1, 0][i] ?? 0; c[k] && c[k].click() }) }); await W(300)
  await p.$$eval('.arp-check input', cs => cs[0] && cs[0].click()); await W(200)
  await scrollTo('.arp'); await W(500)
  await save('24-ai-reply-questions')
  await p.$$eval('.arp .btn.primary', bs => bs[0].click()); await W(4500)
  await p.evaluate(() => { const b = document.querySelector('.reader-body'); const c = document.querySelector('.reader [contenteditable=true]'); c && c.scrollIntoView({ block: 'start' }); b.scrollBy(0, -140) }); await W(600)
  await save('25-ai-written-reply')
})

// ---------------------------------------------------------------- search, live events
shot('26-search', async () => {
  await go('/'); await p.type('input[type=search]', L('seal', 'уплотн')); await p.keyboard.press('Enter'); await W(1800); await quiet(500)
  await clickText('.row', L('Updated price list', 'Обновлённый прайс')); await quiet(1500)
  await save('26-search')
})
shot('27-search-filters', async () => {
  await go('/'); await p.$$eval('.chips .chip', c => c[0].click()); await W(700); await quiet(1200)
  await p.$$eval('.chips .chip', c => c[2].click()); await W(900)
  await save('27-search-filters')
})
shot('28-new-mail-arrives', async () => {
  await go('/'); await W(800); await demo('/demo/push?id=n1'); await W(1600)
  await save('28-new-mail-arrives')
})
shot('29-new-in-conversation', async () => {
  await openLetter('anna@' + D, 'inbox', '', L('Request for quotation', 'Запрос КП'))
  await demo('/demo/push?id=n2'); await W(1800); await scrollReader(0); await W(300)
  await save('29-new-in-conversation')
})
shot('30-calendar-reminder', async () => {
  await go('/'); const r = await p.$$('.rail-btn'); await r[1].click(); await p.mouse.move(1439, 899); await quiet(1200)
  await demo('/demo/reminder'); await W(1500)
  await save('30-calendar-reminder')
  await p.$$eval('.btn', bs => { const x = bs.find(b => /^\s*(Done|Выполнено)\s*$/.test(b.textContent)); x && x.click() }); await W(600)
  await demo('/demo/reminder-done')
})

// ---------------------------------------------------------------- contacts
shot('31-contact-company', async () => {
  await go('/'); const r = await p.$$('.rail-btn'); await r[2].click(); await p.mouse.move(1439, 899); await p.waitForSelector('.crm-item'); await quiet(1200)
  await clickText('.crm-item b', L('Brightline Parts GmbH', 'ООО «Брайтлайн Партс»')); await quiet(1200)
  await save('31-contact-company')
})
shot('32-contact-links', async () => {
  await go('/'); const r = await p.$$('.rail-btn'); await r[2].click(); await p.mouse.move(1439, 899); await p.waitForSelector('.crm-item'); await quiet(1200)
  await clickText('.crm-item b', L('Tidewater Shipyards B.V.', 'Балтийская Верфь')); await quiet(1200)
  await W(300)
  await save('32-contact-links')
})

// ---------------------------------------------------------------- settings
async function settingsTab(i) {
  await go('/'); const r = await p.$$('.rail-btn'); await r[4].click(); await p.mouse.move(1439, 899); await W(900)
  const t = await p.$$('.tabs [role=tab]'); await t[i].click(); await quiet(900)
}
shot('33-settings-appearance', async () => { await settingsTab(0); await save('33-settings-appearance') })
shot('34-settings-signatures', async () => { await settingsTab(1); await save('34-settings-signatures') })
shot('35-settings-notifications', async () => { await settingsTab(2); await save('35-settings-notifications') })
shot('36-settings-templates', async () => { await settingsTab(3); await save('36-settings-templates') })
shot('37-settings-auto-reply', async () => { await settingsTab(4); await save('37-settings-auto-reply') })
shot('38-settings-mail-apps', async () => { await settingsTab(5); await p.$$eval('.modal details summary, details summary', s => s[0] && s[0].click()); await W(500); await save('38-settings-mail-apps') })
shot('39-settings-ai-agents-mcp', async () => { await settingsTab(6); await save('39-settings-ai-agents-mcp') })
shot('40-settings-profile-languages', async () => {
  await settingsTab(7); const s = await p.$$('.modal .sel'); await s[0].click(); await W(700)
  await p.evaluate(() => { const l = document.querySelector('.sel-list'); const o = [...l.querySelectorAll('.sel-opt')].find(x => /Қазақша/.test(x.textContent)); l.scrollTop = o.offsetTop - 60 }); await W(300)
  await save('40-settings-profile-languages')
})

// ---------------------------------------------------------------- admin
const admin = async (path, name, after) => { await go('/admin' + path); await p.waitForSelector('.adm-page'); await quiet(1500); if (after) await after(); await save(name) }
shot('41-admin-overview', () => admin('', '41-admin-overview'))
shot('42-admin-users', () => admin('/users', '42-admin-users'))
shot('43-admin-user-editor', () => admin('/users', '43-admin-user-editor', async () => { await clickText('tr.click', L('Mark Ellison', 'Максим Орлов')); await W(1000) }))
shot('53-admin-user-mailbox-access', () => admin('/users', '53-admin-user-mailbox-access', async () => { await clickText('tr.click', L('Mark Ellison', 'Максим Орлов')); await W(1000); await clickText('.modal .tabs button, .modal [role=tab], .modal button', L('Mailbox access', 'Доступ к ящикам')); await W(700) }))
shot('44-admin-mailboxes', () => admin('/mailboxes', '44-admin-mailboxes'))
shot('45-admin-aliases', () => admin('/aliases', '45-admin-aliases'))
shot('46-admin-domains-dns', () => admin('/domains', '46-admin-domains-dns', async () => { await clickText('.chip', 'nordwave-parts'); await quiet(1800); await p.evaluate(() => document.querySelectorAll('.dns-row')[3].scrollIntoView({ block: 'center' })); await W(400) }))
shot('47-admin-ai-assistant', () => admin('/ai', '47-admin-ai-assistant'))
shot('48-admin-mail-collector', () => admin('/collector', '48-admin-mail-collector'))
shot('49-admin-block-allow-lists', () => admin('/senders', '49-admin-block-allow-lists'))
shot('50-admin-antivirus', () => admin('/antivirus', '50-admin-antivirus'))
shot('51-admin-server', () => admin('/server', '51-admin-server'))
shot('52-admin-audit-log', () => admin('/audit', '52-admin-audit-log'))

// ---------------------------------------------------------------- dark theme, background
shot('60-dark-inbox', async () => {
  await go('/'); await api('/api/settings', { method: 'PUT', body: JSON.stringify({ theme: 'dark' }) })
  await go('/'); await save('60-dark-inbox')
  await openLetter('anna@' + D, 'inbox', '', L('Request for quotation', 'Запрос КП')); await save('61-dark-reading')
  await api('/api/settings', { method: 'PUT', body: JSON.stringify({ theme: 'light' }) })
})
shot('62-background-image', async () => {
  await go('/'); await api('/api/me/background', { method: 'POST', body: JSON.stringify({ background: 'mountains' }) })
  await go('/'); await save('62-background-image')
  await api('/api/me/background', { method: 'POST', body: JSON.stringify({ background: '' }) })
})

// ---------------------------------------------------------------- phone
shot('70-phone', async () => {
  const ph = await b.newPage()
  await ph.evaluateOnNewDocument(() => { try { localStorage.setItem('zm-notify-banner', 'dismissed') } catch {} })
  await ph.setViewport({ width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true })
  const saveP = async name => { let buf = Buffer.from(await ph.screenshot({ type: 'webp', quality: 82 })); if (buf.length > 250 * 1024) buf = Buffer.from(await ph.screenshot({ type: 'webp', quality: 68 })); fs.writeFileSync(`${OUT}/${name}.webp`, buf); console.log('saved', name, Math.round(buf.length / 1024) + 'KB') }
  const quietP = async (ms = 600) => { await ph.waitForFunction(() => !document.querySelector('.veil'), { timeout: 20000 }).catch(() => {}); await W(ms) }
  await ph.goto(HOST + '/', { waitUntil: 'load' }); await ph.waitForSelector('.row'); await quietP(1500)
  await saveP('70-phone-inbox')
  await ph.$$eval('.row', (rs, t) => rs.find(r => r.textContent.includes(t)).click(), L('Request for quotation', 'Запрос КП')); await W(1000); await quietP(1500)
  await saveP('71-phone-reading')
  await ph.goto(HOST + '/', { waitUntil: 'load' }); await ph.waitForSelector('.row'); await quietP(1200)
  await ph.evaluate(() => { const b = [...document.querySelectorAll('button')].find(x => x.classList.contains('compose-btn') || /fab/.test(x.className)); b && b.click() }); await W(1200)
  await ph.type('#c-to', L('femke', 'фролова')); await W(800); await ph.keyboard.press('Enter')
  await ph.type('#c-subj', L('Call tomorrow at 14:00', 'Созвон завтра в 14:00'))
  await ph.evaluate(() => { const el = document.querySelector('.cw [contenteditable=true]'); el.focus(); const r = document.createRange(); r.setStart(el, 0); r.collapse(true); getSelection().removeAllRanges(); getSelection().addRange(r) })
  await ph.keyboard.type(L('Hi Femke, tomorrow at 14:00 works for me. Talk soon!', 'Елена, завтра в 14:00 удобно. До связи!'), { delay: 5 }); await W(800)
  await saveP('72-phone-compose')
  await ph.goto(HOST + '/', { waitUntil: 'load' }); await ph.waitForSelector('.row'); await quietP(1200)
  await ph.evaluate(() => { const b = document.querySelector('.menu-btn, .list-head .icon-btn, [aria-label*="enu"]'); b && b.click() }); await W(1000)
  await saveP('73-phone-menu')
  await ph.close()
})

;(async () => {
  b = await launch(PORT, LANG)
  p = await b.newPage()
  await setup(p, errs)
  await p.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 })
  await p.goto(HOST + '/login', { waitUntil: 'load' })
  await p.evaluate(() => fetch('/demo/reset', { method: 'POST' }))
  await p.evaluate(() => fetch('/demo/reminder-done', { method: 'POST' }))
  await p.setCookie({ name: 'zm_sess', value: 'demo-readme', domain: 'mail.nordwave.example', path: '/' })
  for (const s of shots) {
    if (ONLY.length && !ONLY.some(o => s.name.startsWith(o))) continue
    try { await s.fn() } catch (e) { errs.push(`SHOT ${s.name}: ${e.message.split('\n')[0]}`) }
    // Letters sent or moved in one shot must not change the next ones.
    if (/^(16|20|21|28|29|24)/.test(s.name)) { await demo('/demo/reset'); await demo('/demo/reminder-done') }
  }
  console.log('ERRORS', errs.length); errs.forEach(e => console.log(' ', e))
  await b.close()
})().catch(e => { console.error('FAIL', e); process.exit(1) })
