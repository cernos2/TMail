// Server messages in the user's language. The Go server answers in English; each of its messages
// is listed in locales/server-patterns.json as [key, Go format string]. A format becomes a regular
// expression (each verb such as %s, %d or %q captures a value), and the matching key is looked up in
// the interface dictionary with the captured values as {0}, {1}… Unknown messages stay as they are.
import { t } from '~/composables/i18n'
import patterns from '~/locales/server-patterns.json'

type Pattern = { key: string, full: RegExp, prefix: RegExp | null, nested: boolean[] }

// Go verbs: %v %s %d %q %w %.1f %x… ("%%" is a literal percent sign).
const VERB = /%%|%[-+#0]*\d*(?:\.\d+)?[vTtbcdoOqxXUeEfFgGspw]/g
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\\/]/g, '\\$&')

function compile(key: string, format: string): Pattern {
  let body = ''
  let last = 0
  const nested: boolean[] = []
  let endsWithVerb = false
  for (const m of format.matchAll(VERB)) {
    body += escape(format.slice(last, m.index))
    if (m[0] === '%%') body += '%'
    else {
      body += '(.+?)'
      // Text values (%s %v %w) may themselves be server messages: "Error: " + another message.
      nested.push(/[svw]$/.test(m[0]))
    }
    last = m.index! + m[0].length
    endsWithVerb = last === format.length && m[0] !== '%%'
  }
  body += escape(format.slice(last))
  return {
    key,
    full: new RegExp(`^${body}$`, 's'),
    // A message composed as "<known text>" + err.Error(): the known start is translated, the rest kept.
    prefix: endsWithVerb ? null : new RegExp(`^${body}`, 's'),
    nested,
  }
}

let compiled: Pattern[] | null = null
function list(): Pattern[] {
  if (!compiled) {
    // The most specific formats (the most literal text) are tried first.
    const literal = (f: string) => f.replace(VERB, '').length
    compiled = (patterns as [string, string][])
      .slice()
      .sort((a, b) => literal(b[1]) - literal(a[1]))
      .map(([key, format]) => compile(key, format))
  }
  return compiled
}

function apply(p: Pattern, m: RegExpMatchArray, depth: number): string | null {
  const params: Record<string, string> = {}
  for (let i = 1; i < m.length; i++) {
    const v = m[i] ?? ''
    params[String(i - 1)] = p.nested[i - 1] ? translate(v, depth + 1) : v
  }
  const out = t(p.key, params)
  return out === p.key ? null : out // the key is missing from the dictionary
}

function translate(msg: string, depth: number): string {
  if (!msg || depth > 3) return msg
  const all = list()
  for (const p of all) {
    const m = msg.match(p.full)
    if (m) {
      const out = apply(p, m, depth)
      if (out !== null) return out
    }
  }
  // No whole match: the longest known beginning, ending at a word boundary ("No file" is not the
  // beginning of "No files here").
  let best: { p: Pattern, m: RegExpMatchArray } | null = null
  for (const p of all) {
    if (!p.prefix) continue
    const m = msg.match(p.prefix)
    if (!m || (/[\p{L}\p{N}]$/u.test(m[0]) && /^[\p{L}\p{N}]/u.test(msg.slice(m[0].length)))) continue
    if (!best || m[0].length > best.m[0].length) best = { p, m }
  }
  if (best) {
    const out = apply(best.p, best.m, depth)
    if (out !== null) return out + msg.slice(best.m[0].length)
  }
  return msg
}

export function translateServer(msg: string): string {
  if (typeof msg !== 'string') return msg
  try { return translate(msg, 0) } catch { return msg }
}
