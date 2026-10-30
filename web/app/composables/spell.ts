// Spelling: the server checks words against its Russian and English dictionaries (the text does not
// leave our server); answers are kept here for the session, so a word is asked about once. Wrong words
// are underlined with the CSS Custom Highlight API — the letter's HTML is not touched.
import { api } from '~/composables/api'

type Answer = string[] | null // null: right; list: wrong, with suggestions
const known = new Map<string, Answer>()
const pairs = new Map<string, string[]>() // "thisf eature" → ["this feature"]: a misplaced space
const skipped = new Set<string>() // "Skip": until the page is reloaded
let dict: Set<string> | null = null

// Words: letters, with inner hyphens and apostrophes (well-known, don't).
export const WORD_RE = /[\p{L}\p{M}]+(?:[-'’][\p{L}\p{M}]+)*/gu

export const isWrong = (w: string) => {
  if (skipped.has(w) || dict?.has(w.toLowerCase())) return false
  return Array.isArray(known.get(w))
}
export const suggestions = (w: string) => known.get(w) || []
export const isKnown = (w: string) => known.has(w)
export const pairFixes = (p: string) => pairs.get(p) || []
export const isPairKnown = (p: string) => pairs.has(p)

let dictLoad: Promise<void> | null = null
function loadDict() {
  dictLoad ||= api<{ words: string[] }>('/api/spell/dict')
    .then(r => { dict = new Set(r.words) })
    .catch(() => { dictLoad = null })
  return dictLoad
}

// check asks the server about the words it has not seen yet; resolves when the answers are in.
let offUntil = 0
export async function checkWords(words: string[], pairList: string[] = []) {
  await loadDict()
  const todo = [...new Set(words)].filter(w => !known.has(w)).slice(0, 400)
  const ptodo = [...new Set(pairList)].filter(p => !pairs.has(p)).slice(0, 50)
  if ((!todo.length && !ptodo.length) || Date.now() < offUntil) return
  try {
    const r = await api<{ bad: Record<string, string[]>, pairs?: Record<string, string[]> }>('/api/spell', { method: 'POST', body: { words: todo, pairs: ptodo } })
    for (const w of todo) known.set(w, r.bad[w] || null)
    for (const p of ptodo) pairs.set(p, r.pairs?.[p] || [])
  } catch {
    offUntil = Date.now() + 60_000 // the checker is down: do not hammer it, try again in a minute
  }
}

export function skipWord(w: string) { skipped.add(w) }

export async function addToDict(w: string) {
  dict ||= new Set()
  dict.add(w.toLowerCase())
  await api('/api/spell/dict', { method: 'POST', body: { word: w } })
}
