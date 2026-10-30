// Earlier correspondence quoted in a reply is folded behind a "•••" button, like Gmail: the
// quote starts at a known quote block (Gmail, Outlook, Apple Mail, Thunderbird, Yandex, ours) or at
// an attribution line ("…, Ivan <ivan@firma.ru> wrote:", "-----Original Message-----"), and
// everything after it in the letter is hidden until the button is pressed.
// The Russian and German words below are detection patterns (what other mail programs write into
// letters), not interface text: each has its English equivalent next to it.
import { t } from './i18n'

const QUOTE_SELECTORS = ['[data-tmail-quote]', '.gmail_quote', '#divRplyFwdMsg', '#appendonsend', '.yahoo_quoted', '.moz-cite-prefix',
  'div[name="messageReplySection"]', 'blockquote[type="cite"]', '.mail-quote-collapse', '.moz-forward-container']

// One line that opens a quoted letter.
const reAttribution = new RegExp([
  String.raw`^.{0,240}(пишет|написал|написала|написал\(а\)|wrote|schrieb)\s*:\s*$`,
  String.raw`^.{0,200}<[^<>\s@]+@[^<>\s]+>\s*:\s*$`, // Gmail: «Mon, 26 Oct 2026 at 12:02, Ivan <ivan@firma.ru>:»
  String.raw`^\s*-{2,}\s*(Original Message|Исходное сообщение|Пересылаемое сообщение|Forwarded message|Переадресованное сообщение)\s*-*\s*$`,
  String.raw`^\s*(From|От)\s*:.{3,200}$`,
].join('|'), 'im')

function ownTextBefore(doc: Document, node: Node) {
  const r = doc.createRange()
  r.setStart(doc.body, 0)
  r.setEndBefore(node)
  return r.toString().replace(/\s+/g, '')
}

// A "From:" line counts only as the head of an Outlook-style block (From… Sent/Date… To…).
function isOutlookHead(text: string) {
  return /(From|От)\s*:/i.test(text) && /(Sent|Date|Отправлено|Дата)\s*:/i.test(text)
}

const reDashLine = /^[ \t]*-{8,}[ \t]*$/m
const reHeadField = /(Кому|От|From|To|Тема|Subject|Отправлено|Sent|Дата|Date)\s*:/

// The text following a node in the letter (the lines after a dash line, in the same or next blocks).
function afterText(node: Text) {
  const doc = node.ownerDocument
  const r = doc.createRange()
  r.setStartAfter(node)
  r.setEndAfter(doc.body.lastChild!)
  return r.toString().slice(0, 500)
}

// «19.10.2026, 15:24, Ivan <ivan@firma.ru>» — the head of a quote in Yandex and others (Cyrillic domains too).
const reDatedHead = /(\d{1,2}[./]\d{1,2}[./]\d{2,4}|\d{1,2}:\d{2}).{0,200}@[\w.-]+\.[a-zа-я]{2,}/i

function findStart(doc: Document): Node | null {
  const body = doc.body
  let best: Node | null = null
  const consider = (n: Node | null) => {
    if (!n || n === body) return
    if (best && !(n.compareDocumentPosition(best) & Node.DOCUMENT_POSITION_FOLLOWING)) return // not earlier than what we have
    if (ownTextBefore(doc, n).length < 2) return // nothing of the letter itself would be left
    best = n
  }
  const text = (el: Element) => (el.textContent || '').replace(/\s+/g, ' ').trim()
  for (const sel of QUOTE_SELECTORS) {
    const el = body.querySelector(sel)
    if (!el) continue
    // Apple Mail / Yandex: «… wrote:» right before the blockquote belongs to the quote.
    const prev = el.previousElementSibling
    consider(prev && reAttribution.test(text(prev)) ? prev : el)
  }
  // A blockquote that opens with a dated line naming an address.
  for (const bq of Array.from(body.querySelectorAll('blockquote'))) {
    const head = text(bq).slice(0, 300)
    if (reDatedHead.test(head) || reAttribution.test(head.split(/\n/)[0] || '')) {
      const prev = bq.previousElementSibling
      consider(prev && reAttribution.test(text(prev)) ? prev : bq)
      break
    }
  }
  // Small blocks: an Outlook head (From… Sent… To…) or an attribution line, whatever tags split it.
  for (const el of Array.from(body.querySelectorAll('p,div,span,td,font,b'))) {
    const s = text(el)
    if (!s || s.length > 600) continue
    if ((/^(From|От)\s*:/i.test(s) && isOutlookHead(s)) || (s.length < 300 && reAttribution.test(s) && !/^(From|От)\s*:/i.test(s))) {
      consider(el)
      break
    }
  }
  // Attribution lines inside longer text (plain-text letters), or a line of dashes heading a
  // copied letter («----------------» then To:/Subject:/From:, in English or Russian).
  const walker = doc.createTreeWalker(body, NodeFilter.SHOW_TEXT)
  for (let tn = walker.nextNode() as Text | null; tn; tn = walker.nextNode() as Text | null) {
    const dash = reDashLine.exec(tn.data)
    if (dash) {
      const after = tn.data.slice(dash.index) + ' ' + afterText(tn)
      if (reHeadField.test(after.slice(0, 500))) {
        consider(dash.index > 0 ? tn.splitText(dash.index) : tn)
        break
      }
    }
    const m = reAttribution.exec(tn.data)
    if (!m) continue
    if (/^\s*(From|От)\s*:/i.test(m[0])) {
      const block = tn.data.slice(m.index, m.index + 600)
      if (!isOutlookHead(block)) continue
    }
    const start: Node = m.index > 0 ? tn.splitText(m.index) : tn // the line starts inside a longer text
    consider(start)
    break
  }
  return best
}

// Hides the start node and everything after it (on every level up to <body>); returns a toggle.
function hideFrom(doc: Document, start: Node) {
  const hidden: { el: HTMLElement, display: string }[] = []
  const hide = (n: Node) => {
    let el: HTMLElement
    if (n.nodeType === Node.ELEMENT_NODE) el = n as HTMLElement
    else if (n.nodeType === Node.TEXT_NODE && (n as Text).data.trim()) {
      el = doc.createElement('span')
      n.parentNode!.insertBefore(el, n)
      el.appendChild(n)
    } else return
    hidden.push({ el, display: el.style.display })
    el.style.display = 'none'
  }
  let cur: Node | null = start
  while (cur && cur !== doc.body) {
    const parent: Node | null = cur.parentNode // before hiding: a hidden text node moves into a <span>
    for (let sib: Node | null = cur; sib;) {
      const next: Node | null = sib.nextSibling
      hide(sib)
      sib = next
    }
    // One level up: whatever follows the parent (the parent itself holds the letter's own text).
    cur = null
    for (let p = parent; p && p !== doc.body; p = p.parentNode) {
      if (p.nextSibling) { cur = p.nextSibling; break }
    }
  }
  return (show: boolean) => hidden.forEach(h => { h.el.style.display = show ? h.display : 'none' })
}

export function collapseQuotes(doc: Document, onChange: () => void) {
  if (!doc.body || doc.body.querySelector('.tm-q')) return
  const start = findStart(doc)
  if (!start) return
  const btn = doc.createElement('button')
  btn.type = 'button'
  btn.className = 'tm-q'
  btn.textContent = '•••'
  btn.title = t('mail.quotes.show')
  btn.setAttribute('aria-label', t('mail.quotes.showEarlier'))
  btn.setAttribute('aria-expanded', 'false')
  start.parentNode!.insertBefore(btn, start)
  const toggle = hideFrom(doc, start)
  let open = false
  btn.addEventListener('click', () => {
    open = !open
    toggle(open)
    btn.setAttribute('aria-expanded', String(open))
    btn.title = open ? t('mail.quotes.hide') : t('mail.quotes.show')
    onChange()
  })
}
