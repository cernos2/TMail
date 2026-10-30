// Builds the demo's sample files (PDF, DOCX, XLSX, photos, thumbnails, OCR layers) into demo/assets.
// Needs puppeteer-core and a Chromium; not a dependency of the project:
//   PUPPETEER=/path/to/node_modules/puppeteer-core CHROMIUM=/snap/bin/chromium node demo/tools/make-assets.mjs
// The generated files are committed, so the demo server itself needs nothing but Node.
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { documents, CSS, blocksHtml, sheetsHtml } from './docs.mjs'
import { xlsx, docx } from './zip.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const OUT = path.join(here, '..', 'assets')
const require = createRequire(import.meta.url)
const puppeteer = require(process.env.PUPPETEER || 'puppeteer-core')
const browser = await puppeteer.launch({
  executablePath: process.env.CHROMIUM || '/snap/bin/chromium', headless: true, args: ['--no-sandbox'],
  userDataDir: path.join(os.homedir(), 'snap/chromium/common/pp-demo-assets'),
})
const page = await browser.newPage()
const A4W = 794, A4H = 1123
const write = (rel, buf) => { const f = path.join(OUT, rel); fs.mkdirSync(path.dirname(f), { recursive: true }); fs.writeFileSync(f, buf); console.log('  ', rel, buf.length) }
const html = (body, extra = '') => `<!doctype html><html><head><meta charset="utf-8"><style>${CSS}${extra}</style></head><body>${body}</body></html>`

async function pdfOf(body, extra) {
  await page.setViewport({ width: A4W, height: A4H, deviceScaleFactor: 1 })
  await page.setContent(html(body, extra), { waitUntil: 'load' })
  return Buffer.from(await page.pdf({ format: 'A4', printBackground: true, margin: { top: 0, right: 0, bottom: 0, left: 0 }, pageRanges: '1' }))
}
async function shot(body, { w = A4W, h = A4H, scale = 0.5, type = 'png', quality, extra = '', transparent = false } = {}) {
  await page.setViewport({ width: w, height: h, deviceScaleFactor: scale })
  await page.setContent(html(body, extra), { waitUntil: 'load' })
  await new Promise(r => setTimeout(r, 150))
  return Buffer.from(await page.screenshot({ type, quality, omitBackground: transparent, clip: { x: 0, y: 0, width: w, height: h } }))
}
// Word boxes of the rendered page, as the server's OCR answers them (image pixels).
async function ocrWords(scale, rootSel = 'body') {
  return page.evaluate((scale, rootSel) => {
    const out = []
    const walker = document.createTreeWalker(document.querySelector(rootSel), NodeFilter.SHOW_TEXT)
    const lines = []
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const text = n.textContent
      const re = /\S+/g
      let m
      while ((m = re.exec(text))) {
        const r = document.createRange()
        r.setStart(n, m.index); r.setEnd(n, m.index + m[0].length)
        const b = r.getBoundingClientRect()
        if (!b.width) continue
        const cy = b.top + b.height / 2
        let l = lines.findIndex(y => Math.abs(y - cy) < b.height * 0.45)
        if (l < 0) { lines.push(cy); l = lines.length - 1 }
        out.push({ t: m[0], x: Math.round(b.left * scale), y: Math.round(b.top * scale), w: Math.round(b.width * scale), h: Math.round(b.height * scale), l })
      }
    }
    // Reading order: by line, then left to right; line numbers renumbered top to bottom.
    const order = lines.map((y, i) => [y, i]).sort((a, b) => a[0] - b[0]).map(x => x[1])
    const rank = new Map(order.map((li, k) => [li, k]))
    out.forEach(w => { w.l = rank.get(w.l) })
    out.sort((a, b) => a.l - b.l || a.x - b.x)
    return out
  }, scale, rootSel)
}
const ocrJson = (pages) => JSON.stringify({ pages, text: pages.map(p => { let s = '', l = -1; for (const w of p.words) { s += (l === -1 ? '' : w.l !== l ? '\n' : ' ') + w.t; l = w.l } return s }).join('\n\n') })

// ---------- pictures drawn as SVG (photos, a drawing, logos) ----------
const noise = (id, freq = 0.9, op = 0.18) => `<filter id="${id}"><feTurbulence type="fractalNoise" baseFrequency="${freq}" numOctaves="3" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/><feComponentTransfer><feFuncA type="table" tableValues="0 ${op}"/></feComponentTransfer></filter>`
function castingSvg(variant) {
  const holes = Array.from({ length: 8 }, (_, i) => { const a = i / 8 * Math.PI * 2; return `<circle cx="${(Math.cos(a) * 205).toFixed(1)}" cy="${(Math.sin(a) * 205).toFixed(1)}" r="21" fill="url(#hole)"/>` }).join('')
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="900" viewBox="0 0 1200 900">
<defs>${noise('n1', 0.8, 0.35)}${noise('n2', 2.2, 0.25)}
<radialGradient id="floor" cx="50%" cy="40%" r="80%"><stop offset="0" stop-color="#8d8a84"/><stop offset="1" stop-color="#3f3d3a"/></radialGradient>
<radialGradient id="metal" cx="38%" cy="32%" r="75%"><stop offset="0" stop-color="#9aa0a6"/><stop offset=".45" stop-color="#5f6469"/><stop offset="1" stop-color="#2c2f33"/></radialGradient>
<radialGradient id="bore" cx="55%" cy="60%" r="60%"><stop offset="0" stop-color="#0d0e10"/><stop offset=".8" stop-color="#26292d"/><stop offset="1" stop-color="#4a4e53"/></radialGradient>
<radialGradient id="hole" cx="60%" cy="60%" r="70%"><stop offset="0" stop-color="#111"/><stop offset="1" stop-color="#3a3d41"/></radialGradient>
<radialGradient id="vig" cx="50%" cy="50%" r="72%"><stop offset=".55" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".55"/></radialGradient>
<linearGradient id="rust" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#8a4b22" stop-opacity=".0"/><stop offset=".7" stop-color="#8a4b22" stop-opacity=".35"/><stop offset="1" stop-color="#5a2e12" stop-opacity=".55"/></linearGradient>
</defs>
<rect width="1200" height="900" fill="url(#floor)"/><rect width="1200" height="900" filter="url(#n1)"/>
<g transform="translate(${variant ? 560 : 610} ${variant ? 470 : 440}) rotate(${variant ? -18 : 9}) scale(${variant ? 1.08 : 1})">
<ellipse cx="22" cy="34" rx="300" ry="292" fill="#000" opacity=".45" filter="blur(2px)"/>
<clipPath id="cc"><circle r="290"/></clipPath>
<circle r="290" fill="url(#metal)"/><circle r="290" fill="url(#rust)"/><rect x="-300" y="-300" width="600" height="600" filter="url(#n2)" clip-path="url(#cc)"/>
<circle r="250" fill="none" stroke="#2d3034" stroke-width="3" opacity=".6"/>
${holes}
<circle r="150" fill="#4b4f54"/><circle r="150" fill="url(#rust)" opacity=".7"/><circle r="122" fill="url(#bore)"/>
${variant ? '<path d="M 70 -180 q 30 25 18 60 q -10 25 6 48" stroke="#1a1b1d" stroke-width="3.5" fill="none" opacity=".85"/>' : ''}
</g>
<rect x="${variant ? 120 : 880}" y="${variant ? 690 : 110}" width="170" height="64" rx="4" fill="#e9e4d4" transform="rotate(${variant ? -6 : 4} 200 700)"/>
<text x="${variant ? 140 : 900}" y="${variant ? 732 : 152}" font-family="DejaVu Sans Mono" font-size="26" fill="#2b2b2b" transform="rotate(${variant ? -6 : 4} 200 700)">HV-${variant ? '1187' : '1186'}</text>
<rect width="1200" height="900" fill="url(#vig)"/></svg>`
}
function drawingSvg() {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="720" viewBox="0 0 960 720"><rect width="960" height="720" fill="#f7f9fc"/>
<g stroke="#c9d6e8" stroke-width="1">${Array.from({ length: 24 }, (_, i) => `<line x1="${i * 40}" y1="0" x2="${i * 40}" y2="720"/>`).join('')}${Array.from({ length: 18 }, (_, i) => `<line x1="0" y1="${i * 40}" x2="960" y2="${i * 40}"/>`).join('')}</g>
<g fill="none" stroke="#1f3b66" stroke-width="3"><rect x="150" y="250" width="330" height="220" rx="16"/><circle cx="560" cy="360" r="120"/><circle cx="560" cy="360" r="62"/>
<rect x="680" y="320" width="170" height="80"/><rect x="230" y="190" width="140" height="60"/><line x1="100" y1="470" x2="880" y2="470"/><rect x="120" y="470" width="740" height="40"/></g>
<g stroke="#1f3b66" stroke-width="1.5" stroke-dasharray="10 6"><line x1="440" y1="360" x2="900" y2="360"/><line x1="560" y1="200" x2="560" y2="520"/></g>
<g fill="#1f3b66" font-family="DejaVu Sans" font-size="18"><text x="150" y="570">KP-300/45 · ASSEMBLY · REV C</text><text x="150" y="600" font-size="14">DN125 / DN100 · 22 kW · 1:10</text></g>
<rect x="640" y="560" width="260" height="110" fill="none" stroke="#1f3b66" stroke-width="2"/><text x="656" y="596" font-family="DejaVu Sans" font-size="16" fill="#1f3b66">NORDWAVE ENGINEERING</text><text x="656" y="626" font-family="DejaVu Sans" font-size="13" fill="#1f3b66">DWG NW-KP300-0001</text><text x="656" y="652" font-family="DejaVu Sans" font-size="13" fill="#1f3b66">SHEET 1/14</text></svg>`
}

for (const lang of ['en', 'ru']) {
  console.log(lang)
  const D = documents(lang)
  const L = (en, ru) => lang === 'ru' ? ru : en
  // PDFs with a text layer, and their first page as a thumbnail.
  for (const k of ['invoice', 'offer', 'spec', 'po']) {
    write(`${lang}/${k}.pdf`, await pdfOf(D[k]))
    write(`${lang}/${k}.thumb.png`, await shot(D[k], { scale: 0.5 }))
  }
  // DOCX files and their PDF previews.
  for (const [k, blocks] of [['contract', D.contractBlocks], ['manual', D.manualBlocks]]) {
    write(`${lang}/${k}.docx`, docx(blocks))
    write(`${lang}/${k}.pdf`, await pdfOf(blocksHtml(blocks)))
    write(`${lang}/${k}.thumb.png`, await shot(blocksHtml(blocks), { scale: 0.5 }))
  }
  // Price list: XLSX, its "LibreOffice" HTML for the sheet preview, a PDF and a thumbnail.
  write(`${lang}/pricelist.xlsx`, xlsx(D.sheets))
  const sheetHtml = sheetsHtml(D.sheets, lang)
  write(`${lang}/pricelist.html`, Buffer.from(sheetHtml))
  const sheetPage = `<div class="page"><h1>${D.sheets[0].name}</h1>${sheetHtml.replace(/^.*?<body>/s, '').replace(/<h1>.*?<\/h1>/g, '').replace(/<\/table>.*$/s, '</table>').replace('<table', '<table style="font-size:9pt" ')}</div>`
  write(`${lang}/pricelist.pdf`, await pdfOf(sheetPage))
  write(`${lang}/pricelist.thumb.png`, await shot(sheetPage, { scale: 0.5 }))

  // Scanned delivery note: a JPEG page (a "scan"), an image-only PDF, and the OCR words.
  const scanCss = `html,body{background:#e4e1da}.page.scan{background:#f4f1ea;filter:contrast(1.05)}.hand{font:italic 22pt/1 "URW Chancery L","Z003",cursive;color:#1b3a8a;display:inline-block;transform:rotate(-4deg);margin-top:2mm}
.stamp{position:absolute;right:30mm;top:178mm;border:3px solid #2649a8;color:#2649a8;font:700 15pt/1.25 "DejaVu Sans",sans-serif;padding:3mm 6mm;transform:rotate(-12deg);opacity:.75;text-align:center;border-radius:3mm}
.grain{position:absolute;inset:0;pointer-events:none;background-image:url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns='http://www.w3.org/2000/svg' width='300' height='300'>${noise('g', 1.4, 0.16)}<rect width='300' height='300' filter='url(#g)'/></svg>`)}")}`
  const scanBody = D.scan.replace('</div>\n', '</div>\n') + ''
  const SCALE = 1.5625 // A4 at 150 dpi
  await page.setViewport({ width: A4W, height: A4H, deviceScaleFactor: SCALE })
  await page.setContent(html(`<div style="transform:rotate(-0.6deg) scale(.985);transform-origin:50% 50%">${scanBody}</div><div class="grain"></div>`, scanCss), { waitUntil: 'load' })
  await new Promise(r => setTimeout(r, 200))
  const words = await ocrWords(SCALE, '.page')
  const jpg = Buffer.from(await page.screenshot({ type: 'jpeg', quality: 72, clip: { x: 0, y: 0, width: A4W, height: A4H } }))
  write(`${lang}/scan.page1.jpg`, jpg)
  write(`${lang}/scan.ocr.json`, Buffer.from(ocrJson([{ w: Math.round(A4W * SCALE), h: Math.round(A4H * SCALE), words }])))
  write(`${lang}/scan.pdf`, await pdfOf(`<img src="data:image/jpeg;base64,${jpg.toString('base64')}" style="display:block;width:210mm;height:297mm">`))
  write(`${lang}/scan.thumb.png`, await shot(`<img src="data:image/jpeg;base64,${jpg.toString('base64')}" style="display:block;width:${A4W}px;height:${A4H}px">`, { scale: 0.5 }))

  // A photo of a pump nameplate (text to recognise).
  const plate = `<div id="ph" style="width:1200px;height:900px;position:relative;overflow:hidden;background:radial-gradient(ellipse at 40% 35%,#6b7178,#2e3236 75%)">
<svg width="1200" height="900" style="position:absolute;inset:0"><defs>${noise('a', 0.7, 0.3)}</defs><rect width="1200" height="900" filter="url(#a)"/></svg>
<div id="plate" style="position:absolute;left:210px;top:170px;width:780px;height:520px;border-radius:14px;background:linear-gradient(115deg,#d9dde1,#b9bfc6 40%,#e3e6ea 55%,#aeb4bb);box-shadow:0 18px 40px rgba(0,0,0,.55),inset 0 0 0 3px #8d949c;font-family:'DejaVu Sans Mono',monospace;color:#1d2228;padding:44px 56px">
<div style="font:700 44px/1 'DejaVu Sans',sans-serif;letter-spacing:4px">${L('KESTREL', 'KESTREL')}</div>
<div style="font-size:20px;margin:6px 0 26px;letter-spacing:2px">${L('PUMP SYSTEMS · HOUSTON TX', 'PUMP SYSTEMS · АЛМАТЫ')}</div>
<div style="font-size:27px;line-height:1.6">${L('TYPE KP-200/38<br>SERIAL KP2-24-08817<br>Q 90 m³/h &nbsp; H 38 m<br>P 15 kW &nbsp; n 2950 rpm<br>YEAR 2024 &nbsp; IP55', 'ТИП КП-200/38<br>ЗАВ. № KP2-24-08817<br>Q 90 м³/ч &nbsp; H 38 м<br>P 15 кВт &nbsp; n 2950 об/мин<br>ГОД 2024 &nbsp; IP55')}</div>
${[[22, 22], [740, 22], [22, 480], [740, 480]].map(([x, y]) => `<i style="position:absolute;left:${x}px;top:${y}px;width:18px;height:18px;border-radius:50%;background:radial-gradient(circle at 35% 35%,#f4f6f8,#6d737a)"></i>`).join('')}
</div><div style="position:absolute;inset:0;background:radial-gradient(ellipse at 50% 50%,transparent 55%,rgba(0,0,0,.5))"></div></div>`
  await page.setViewport({ width: 1200, height: 900, deviceScaleFactor: 1 })
  await page.setContent(html(plate, 'body{margin:0}'), { waitUntil: 'load' })
  await new Promise(r => setTimeout(r, 150))
  const pw = await ocrWords(1, '#plate')
  write(`${lang}/nameplate.jpg`, Buffer.from(await page.screenshot({ type: 'jpeg', quality: 82 })))
  write(`${lang}/nameplate.ocr.json`, Buffer.from(ocrJson([{ w: 1200, h: 900, words: pw }])))

  // Newsletter pictures (shown only after "Show images").
  write(`${lang}/news-banner.png`, await shot(`<div style="width:600px;height:200px;background:linear-gradient(120deg,#0b3d91,#0a7fb5 60%,#22c3c9);color:#fff;font-family:'DejaVu Sans',sans-serif;padding:34px 36px;position:relative;overflow:hidden">
<div style="font-size:15px;letter-spacing:3px;opacity:.85">${L('PUMP INDUSTRY WEEKLY · ISSUE 412', 'НАСОСНЫЙ ВЕСТНИК · ВЫПУСК 412')}</div><div style="font:700 34px/1.15 'DejaVu Sans',sans-serif;margin-top:14px;max-width:420px">${L('5 trends in seawater pumps for 2027', '5 трендов в насосах для морской воды')}</div>
<svg width="260" height="260" style="position:absolute;right:-40px;top:-30px;opacity:.35"><circle cx="130" cy="130" r="110" fill="none" stroke="#fff" stroke-width="16"/><circle cx="130" cy="130" r="50" fill="#fff"/>${Array.from({ length: 6 }, (_, i) => `<path d="M130 130 L${130 + 100 * Math.cos(i)} ${130 + 100 * Math.sin(i)}" stroke="#fff" stroke-width="12"/>`).join('')}</svg></div>`, { w: 600, h: 200, scale: 1 }))
  write(`${lang}/news-product.png`, await shot(`<div style="width:260px;height:180px;background:linear-gradient(160deg,#eef3f8,#d5dee9);display:grid;place-items:center"><svg width="200" height="140"><rect x="20" y="60" width="90" height="56" rx="10" fill="#2f5d8a"/><circle cx="140" cy="86" r="44" fill="#3f78ad"/><circle cx="140" cy="86" r="18" fill="#1d3b5c"/><rect x="10" y="116" width="180" height="10" fill="#6b7d90"/></svg></div>`, { w: 260, h: 180, scale: 1 }))
}

// ---------- common: photos of castings, the drawing, our logo, a background ----------
console.log('common')
for (const [i, v] of [[1, 0], [2, 1]]) {
  write(`common/casting${i}.jpg`, await shot(castingSvg(v), { w: 1200, h: 900, scale: 1, type: 'jpeg', quality: 80, extra: 'body{margin:0}' }))
}
write('common/drawing.jpg', await shot(drawingSvg(), { w: 960, h: 720, scale: 1, type: 'jpeg', quality: 82, extra: 'body{margin:0}' }))
write('common/logo.png', await shot(`<div style="display:flex;align-items:center;gap:10px;height:56px;padding:0 6px;font:700 30px/1 'DejaVu Sans',sans-serif;letter-spacing:3px;color:#0a3d8f"><svg width="44" height="44" viewBox="0 0 44 44"><rect width="44" height="44" rx="10" fill="#0a4ff5"/><path d="M8 28 q7 -12 14 0 t14 0" stroke="#fff" stroke-width="4" fill="none" stroke-linecap="round"/><path d="M8 19 q7 -12 14 0 t14 0" stroke="#9cc3ff" stroke-width="4" fill="none" stroke-linecap="round"/></svg>NORDWAVE</div>`, { w: 260, h: 56, scale: 2, transparent: true, extra: 'html,body{background:transparent}' }))
write('common/background.jpg', await shot(`<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="900"><defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#5b86c5"/><stop offset=".6" stop-color="#f3c7a0"/><stop offset="1" stop-color="#f7dcc0"/></linearGradient><linearGradient id="w" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#3d6ea8"/><stop offset="1" stop-color="#173a63"/></linearGradient>${noise('bn', 0.6, 0.12)}</defs>
<rect width="1600" height="900" fill="url(#s)"/><circle cx="1160" cy="520" r="70" fill="#fff4d6" opacity=".9"/><path d="M0 560 L180 470 L340 520 L520 420 L700 500 L860 430 L1040 510 L1240 440 L1420 500 L1600 460 L1600 900 L0 900Z" fill="#46597a" opacity=".75"/>
<rect y="600" width="1600" height="300" fill="url(#w)"/>${Array.from({ length: 40 }, (_, i) => `<rect x="${(i * 197) % 1560}" y="${610 + (i * 53) % 260}" width="${40 + (i * 31) % 90}" height="2" fill="#fff" opacity="${0.15 + (i % 5) / 20}"/>`).join('')}<rect width="1600" height="900" filter="url(#bn)"/></svg>`, { w: 1600, h: 900, scale: 1, type: 'jpeg', quality: 80, extra: 'body{margin:0}' }))

await browser.close()
console.log('done')
