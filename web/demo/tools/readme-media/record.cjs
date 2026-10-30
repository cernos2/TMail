// Demo video and short loops from the demo server: node record.cjs <port> <tour|newmail|spell|aireply|undo>
const { W, launch, setup, HOST, fs } = require('./lib.cjs')
const { execFileSync } = require('child_process')
const PORT = process.argv[2], MODE = process.argv[3]
const OUTDIR = '/work/work_mail/docs/video'
const TMP = `${require('os').tmpdir()}/tmail-frames-${MODE}`
const D = 'nordwave.example'
const errs = []
let p, cdp, frames = [], recording = false

const CURSOR = () => {
  const css = `#demo-cursor{position:fixed;left:0;top:0;width:22px;height:22px;z-index:2147483647;pointer-events:none;transform:translate(-3px,-2px);transition:transform .05s}
#demo-cursor svg{filter:drop-shadow(0 1px 2px rgba(0,0,0,.45))}
.demo-click{position:fixed;width:34px;height:34px;margin:-17px 0 0 -17px;border-radius:50%;border:3px solid rgba(10,79,245,.75);z-index:2147483646;pointer-events:none;animation:demoClick .45s ease-out forwards}
@keyframes demoClick{from{transform:scale(.3);opacity:1}to{transform:scale(1.25);opacity:0}}`
  const add = () => {
    if (document.getElementById('demo-cursor')) return
    const st = document.createElement('style'); st.textContent = css; document.head.appendChild(st)
    const c = document.createElement('div'); c.id = 'demo-cursor'
    c.innerHTML = '<svg width="22" height="22" viewBox="0 0 22 22"><path d="M3 2 L3 18 L7.5 13.8 L10.6 20.4 L13.4 19.1 L10.3 12.7 L16.4 12.6 Z" fill="#fff" stroke="#111" stroke-width="1.4" stroke-linejoin="round"/></svg>'
    const pos = window.__demoPos || { x: -40, y: -40 }
    c.style.left = pos.x + 'px'; c.style.top = pos.y + 'px'
    document.body.appendChild(c)
  }
  document.addEventListener('mousemove', e => { window.__demoPos = { x: e.clientX, y: e.clientY }; try { sessionStorage.setItem('demoPos', JSON.stringify(window.__demoPos)) } catch {} const c = document.getElementById('demo-cursor'); if (c) { c.style.left = e.clientX + 'px'; c.style.top = e.clientY + 'px' } }, true)
  document.addEventListener('mousedown', e => { const r = document.createElement('div'); r.className = 'demo-click'; r.style.left = e.clientX + 'px'; r.style.top = e.clientY + 'px'; document.body.appendChild(r); setTimeout(() => r.remove(), 500) }, true)
  try { window.__demoPos = JSON.parse(sessionStorage.getItem('demoPos') || 'null') || undefined } catch {}
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', add); else add()
  const watch = () => new MutationObserver(add).observe(document.body, { childList: true })
  if (document.body) watch(); else document.addEventListener('DOMContentLoaded', watch)
}

let mx = 640, my = 400
const SLOW = process.argv[3] === 'tour' ? 1.45 : 1
async function moveTo(x, y, steps) {
  const d = Math.hypot(x - mx, y - my)
  const n = steps || Math.max(8, Math.min(40, Math.round(d / 18)))
  for (let i = 1; i <= n; i++) {
    const k = i / n, e = k < 0.5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2
    await p.mouse.move(mx + (x - mx) * e, my + (y - my) * e)
    await W(14)
  }
  mx = x; my = y
}
async function centerOf(sel, text) {
  const r = await p.evaluate((sel, text) => {
    const els = [...document.querySelectorAll(sel)].filter(x => x.offsetParent !== null && (!text || x.textContent.includes(text)))
    const el = els[0]; if (!el) return null
    el.scrollIntoView({ block: 'nearest' })
    const b = el.getBoundingClientRect(); return { x: b.left + b.width / 2, y: b.top + b.height / 2 }
  }, sel, text)
  if (!r) throw new Error(`no ${sel} ${text || ''}`)
  return r
}
async function click(sel, text, pause = 350) {
  const c = await centerOf(sel, text)
  await moveTo(c.x, c.y); await W(180)
  await p.mouse.down(); await W(70); await p.mouse.up()
  await W(pause * SLOW)
}
async function hover(sel, text, pause = 600) { const c = await centerOf(sel, text); await moveTo(c.x, c.y); await W(pause * SLOW) }
const quiet = async (ms = 400) => { await p.waitForFunction(() => !document.querySelector('.veil'), { timeout: 20000 }).catch(() => {}); await W(ms) }
const demo = path => p.evaluate(path => fetch(path, { method: 'POST' }).then(r => r.json()), path)
const type = (text, delay = 55) => p.keyboard.type(text, { delay })
async function wheel(sel, dy, steps = 8) { const c = await centerOf(sel); await moveTo(c.x, c.y); for (let i = 0; i < steps; i++) { await p.mouse.wheel({ deltaY: dy / steps }); await W(40) } }

async function startRec() {
  frames = []
  cdp.on('Page.screencastFrame', async f => {
    if (recording) frames.push({ data: f.data, t: f.metadata.timestamp })
    try { await cdp.send('Page.screencastFrameAck', { sessionId: f.sessionId }) } catch {}
  })
  recording = true
  await cdp.send('Page.startScreencast', { format: 'jpeg', quality: 82, maxWidth: 1280, maxHeight: 800, everyNthFrame: 1 })
}
async function stopRec() {
  await W(600)
  recording = false
  await cdp.send('Page.stopScreencast')
  fs.rmSync(TMP, { recursive: true, force: true }); fs.mkdirSync(TMP, { recursive: true })
  let list = ''
  frames.forEach((f, i) => {
    const name = `${TMP}/f${String(i).padStart(5, '0')}.jpg`
    fs.writeFileSync(name, Buffer.from(f.data, 'base64'))
    const dur = i + 1 < frames.length ? Math.max(0.001, frames[i + 1].t - f.t) : 0.8
    list += `file '${name}'\nduration ${dur.toFixed(4)}\n`
  })
  list += `file '${TMP}/f${String(frames.length - 1).padStart(5, '0')}.jpg'\n`
  fs.writeFileSync(`${TMP}/list.txt`, list)
  console.log('frames', frames.length, 'seconds', (frames[frames.length - 1].t - frames[0].t).toFixed(1))
}
function encodeMp4(out) {
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', `${TMP}/list.txt`, '-vf', 'fps=25,scale=1280:800:flags=lanczos,format=yuv420p',
    '-c:v', 'libx264', '-preset', 'slow', '-crf', '24', '-tune', 'stillimage', '-movflags', '+faststart', out])
  console.log(out, Math.round(fs.statSync(out).size / 1024) + 'KB')
}
function encodeWebp(out, speed = 1) {
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', `${TMP}/list.txt`, '-vf', `setpts=PTS/${speed},fps=12,scale=960:-1:flags=lanczos`,
    '-c:v', 'libwebp_anim', '-lossless', '0', '-quality', '62', '-compression_level', '6', '-loop', '0', out])
  console.log(out, Math.round(fs.statSync(out).size / 1024) + 'KB')
}

async function openApp(path = '/') {
  await p.goto(HOST + path, { waitUntil: 'load' }); await p.waitForSelector('.row', { timeout: 20000 }); await quiet(1200)
  await p.mouse.move(mx, my)
}

const modes = {
  async tour() {
    await p.deleteCookie({ name: 'zm_sess', domain: 'mail.nordwave.example' })
    await p.goto(HOST + '/login', { waitUntil: 'load' }); await p.waitForSelector('#login'); await W(400)
    await startRec()
    await W(900)
    await click('#login'); await type('anna', 110)
    await click('#password'); await type('correct-horse', 70)
    await click('button[type=submit]', '', 200)
    await p.waitForSelector('.row'); await quiet(1500)
    // one login, many mailboxes
    await hover('.nav-item.acct', 'anna@', 500); await hover('.nav-item.acct', 'sales@', 500); await hover('.nav-item.acct', 'support@', 400); await hover('.nav-item.acct', 'gmail', 700)
    await click('.nav-item.acct', 'sales@', 1500)
    await hover('.row', 'PO 4471', 900)
    await click('.nav-item.acct', 'All mailboxes', 1500)
    // a conversation with the counterparty box
    await click('.row', 'Request for quotation', 2200)
    await hover('.party-box', '', 1600)
    await hover('.thread', 'Femke', 800)
    // attachment preview
    await click('.file-pill', 'Nordwave_Offer', 2600)
    await wheel('.pv-doc', 500); await W(1200)
    await p.keyboard.press('Escape'); await W(800)
    // reply with AI
    await wheel('.reader-body', 900); await W(600)
    await click('.reply-bar .btn.ai', '', 2800)
    const chips = ['Yes, 4 weeks', '6% with 50% upfront', 'Accept', 'At cost', '14:00']
    for (const c of chips) await click('.arp .chip', c, 350)
    await click('.arp .btn.primary', '', 3800)
    await wheel('.reader-body', 400); await W(1500)
    // templates
    await click('.tb-btn', 'Templates', 1600)
    await p.keyboard.press('Escape'); await W(500)
    // send with undo
    await click('#reader-dock .btn.primary, .composer .btn.primary', 'Send', 2500)
    await click('.toast button', 'Undo', 2000)
    await click('.cw .icon-btn:last-child', '', 1000).catch(() => {})
    // new mail arrives
    await moveTo(700, 450)
    await demo('/demo/push?id=n1'); await W(2500)
    await hover('.toast', 'New message', 1500)
    // calendar reminder
    await click('.rail-btn[aria-label="Calendar"]', '', 1500)
    await demo('/demo/reminder'); await W(2200)
    await hover('.rem', '', 800)
    await click('.rem .btn', 'Done', 1400)
    // contact card
    await click('.rail-btn[aria-label="Contacts"]', '', 1500)
    await click('.crm-item b', 'Brightline Parts GmbH', 1800)
    await wheel('.crm-card-scroll', 500); await W(1200)
    // administration
    await click('a.rail-btn', '', 2500)
    await quiet(800)
    await hover('.stat', 'mailboxes', 700); await hover('.svc-item', 'ClamAV', 900)
    await click('.nav-item', 'Domains and DNS', 2200)
    await click('.nav-item', 'Antivirus', 2200)
    await click('.look-btn', '', 2000)
    // switch the interface language
    await p.waitForSelector('.row'); await quiet(600)
    await click('.rail-btn[aria-label="Settings and appearance"]', '', 1000)
    await click('.tabs [role=tab]', 'Profile', 900)
    await click('.modal .sel', '', 700)
    await type('Рус', 120); await W(500); await p.keyboard.press('Enter'); await W(600)
    await click('.modal .btn.primary', 'Save', 2200)
    await p.keyboard.press('Escape'); await W(2500)
    await stopRec(); encodeMp4(`${OUTDIR}/tmail-tour.mp4`)
  },
  async newmail() {
    await openApp('/'); await moveTo(760, 300)
    await startRec(); await W(1200)
    await demo('/demo/push?id=n1'); await W(3200)
    await hover('.row', 'Tidewater seals', 1000)
    await stopRec(); encodeWebp(`${OUTDIR}/new-mail.webp`)
  },
  async spell() {
    await openApp('/'); await p.click('.compose-btn'); await W(900)
    await p.type('#c-subj', 'Customer portal')
    await p.evaluate(() => { const el = document.querySelector('.cw [contenteditable=true]'); el.focus(); const r = document.createRange(); r.selectNodeContents(el.firstElementChild); getSelection().removeAllRanges(); getSelection().addRange(r) })
    await startRec(); await W(400)
    await type('Our new portal will make thisf eature available next week.', 45)
    await p.mouse.move(5, 5); await W(2200)
    const r = await p.evaluate(() => { const el = document.querySelector('.cw [contenteditable=true]'); const tw = document.createTreeWalker(el, NodeFilter.SHOW_TEXT); for (let n = tw.nextNode(); n; n = tw.nextNode()) { const i = n.data.indexOf('thisf eature'); if (i >= 0) { const g = document.createRange(); g.setStart(n, i); g.setEnd(n, i + 12); const b = g.getBoundingClientRect(); return { x: b.left + b.width / 2, y: b.top + b.height / 2 } } } })
    mx = 900; my = 600; await moveTo(r.x, r.y); await W(1500)
    await click('.spell-tip button.first', '', 1500)
    await stopRec(); encodeWebp(`${OUTDIR}/spell-check.webp`)
  },
  async aireply() {
    await openApp('/'); await click('.row', 'Request for quotation', 2000)
    await p.evaluate(() => { const b = document.querySelector('.reader-body'); b.scrollTop = b.scrollHeight }); await W(600)
    await startRec(); await W(300)
    await click('.reply-bar .btn.ai', '', 2200)
    for (const c of ['Yes, 4 weeks', '6% with 50% upfront', 'At cost']) await click('.arp .chip', c, 200)
    await click('.arp .btn.primary', '', 3600)
    await stopRec(); encodeWebp(`${OUTDIR}/reply-with-ai.webp`, 1.15)
  },
  async undo() {
    await openApp('/'); await p.click('.compose-btn'); await W(900)
    await p.type('#c-to', 'femke'); await W(800); await p.keyboard.press('Enter')
    await p.type('#c-subj', 'Call tomorrow at 14:00')
    await p.evaluate(() => { const el = document.querySelector('.cw [contenteditable=true]'); el.focus(); const r = document.createRange(); r.selectNodeContents(el.firstElementChild); getSelection().removeAllRanges(); getSelection().addRange(r) })
    await p.keyboard.type('Hi Femke, tomorrow at 14:00 works for me.', { delay: 3 }); await W(400)
    await startRec(); await W(500)
    await click('.cw .btn.primary', 'Send', 2600)
    await click('.toast button', 'Undo', 2200)
    await stopRec(); encodeWebp(`${OUTDIR}/send-undo.webp`)
  },
}

;(async () => {
  const b = await launch(PORT, 'rec-' + MODE)
  p = await b.newPage()
  await setup(p, errs)
  await p.evaluateOnNewDocument(CURSOR)
  await p.setViewport({ width: 1280, height: 800, deviceScaleFactor: 1 })
  cdp = await p.target().createCDPSession()
  await p.goto(HOST + '/login', { waitUntil: 'load' })
  await demo('/demo/reset'); await demo('/demo/reminder-done')
  await p.setCookie({ name: 'zm_sess', value: 'demo-video', domain: 'mail.nordwave.example', path: '/' })
  try { await modes[MODE]() } catch (e) { errs.push('MODE ' + e.message) }
  await demo('/demo/reset')
  console.log('ERRORS', errs.length); errs.forEach(e => console.log(' ', e))
  await b.close()
})().catch(e => { console.error('FAIL', e); process.exit(1) })
