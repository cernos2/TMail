// Addresses and sites written as plain text in a letter ("write to ivan@firma.ru", "www.firma.ru")
// become links, like in Gmail. Text already inside a link, styles and code are not touched.

const RE = /(?<![\p{L}\p{N}._%+\/@-])((?:https?:\/\/|www\.)[^\s<>"'«»]+[^\s<>"'«».,;:!?)\]]|[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)*\.\p{L}{2,})/gu
const SKIP = new Set(['A', 'STYLE', 'SCRIPT', 'TEXTAREA', 'BUTTON', 'CODE', 'PRE', 'TITLE'])

export function linkify(doc: Document) {
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT, {
    acceptNode(n) {
      for (let p = n.parentElement; p && p !== doc.body; p = p.parentElement) if (SKIP.has(p.tagName)) return NodeFilter.FILTER_REJECT
      return /[@.]/.test(n.nodeValue || '') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP
    },
  })
  const nodes: Text[] = []
  for (let n = walker.nextNode(); n; n = walker.nextNode()) nodes.push(n as Text)
  for (const node of nodes) {
    const text = node.nodeValue || ''
    RE.lastIndex = 0
    if (!RE.test(text)) continue
    RE.lastIndex = 0
    const frag = doc.createDocumentFragment()
    let at = 0
    for (const m of text.matchAll(RE)) {
      const s = m[0], i = m.index!
      if (i > at) frag.append(text.slice(at, i))
      const a = doc.createElement('a')
      a.textContent = s
      a.setAttribute('href', s.includes('@') && !/^(https?:|www\.)/i.test(s) ? 'mailto:' + s : /^www\./i.test(s) ? 'https://' + s : s)
      a.setAttribute('rel', 'noopener noreferrer')
      frag.append(a)
      at = i + s.length
    }
    if (at < text.length) frag.append(text.slice(at))
    node.replaceWith(frag)
  }
}

// mailto:ivan@firma.ru?subject=…&cc=… → its parts.
export function parseMailto(href: string) {
  const [addr, q = ''] = href.replace(/^mailto:/i, '').split('?')
  const p = new URLSearchParams(q)
  return { to: decodeURIComponent(addr || ''), cc: p.get('cc') || '', subject: p.get('subject') || '', body: p.get('body') || '' }
}
