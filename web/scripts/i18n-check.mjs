// Checks the translations: every key used in the code exists in en.json, no key is unused,
// every language has the same placeholders as English. With --merge, first folds
// app/locales/parts/*.json ({ key: { ru, en } }) into ru.json and en.json and deletes the parts.
// Usage: node scripts/i18n-check.mjs [--merge]
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..')
const dir = path.join(root, 'app/locales')
const read = f => JSON.parse(fs.readFileSync(f, 'utf8'))
const write = (f, d) => fs.writeFileSync(f, JSON.stringify(Object.fromEntries(Object.entries(d).sort(([a], [b]) => a.localeCompare(b))), null, 1) + '\n')
let problems = 0
const problem = m => { problems++; console.log('  ' + m) }

if (process.argv.includes('--merge')) {
  const ru = read(path.join(dir, 'ru.json')), en = read(path.join(dir, 'en.json'))
  const partsDir = path.join(dir, 'parts')
  for (const f of fs.existsSync(partsDir) ? fs.readdirSync(partsDir).filter(f => f.endsWith('.json')) : []) {
    for (const [k, v] of Object.entries(read(path.join(partsDir, f)))) {
      if (en[k] !== undefined && (en[k] !== v.en || ru[k] !== v.ru)) problem(`${f}: ${k} differs from an existing key: "${en[k]}" vs "${v.en}"`)
      else { ru[k] = v.ru; en[k] = v.en }
    }
  }
  write(path.join(dir, 'ru.json'), ru); write(path.join(dir, 'en.json'), en)
  console.log(`merged: ${Object.keys(en).length} keys`)
}

const en = read(path.join(dir, 'en.json'))
// Keys in the code: t('a.b'…), t("a.b"…), and keys written as 'area.key' strings next to t( calls.
const used = new Set(), files = []
const walk = d => { for (const e of fs.readdirSync(d, { withFileTypes: true })) { const p = path.join(d, e.name); if (e.isDirectory()) { if (e.name !== 'locales') walk(p) } else if (/\.(vue|ts)$/.test(e.name)) files.push(p) } }
walk(path.join(root, 'app'))
for (const f of files) {
  const s = fs.readFileSync(f, 'utf8')
  for (const m of s.matchAll(/\bt\(\s*['"`]([a-zA-Z0-9_.]+)['"`]/g)) {
    used.add(m[1])
    if (!(m[1] in en)) problem(`${path.relative(root, f)}: missing key ${m[1]}`)
  }
  for (const m of s.matchAll(/['"`]((?:common|date|unit|[a-z]+)\.[a-zA-Z0-9_.]+)['"`]/g)) if (m[1] in en) used.add(m[1])
}
const dynamic = Object.keys(en).filter(k => !used.has(k))
if (dynamic.length) console.log(`keys not found literally in code (built dynamically or unused): ${dynamic.length}\n  ${dynamic.slice(0, 40).join(', ')}${dynamic.length > 40 ? ' …' : ''}`)

// Placeholders at any depth, but not the words inside plural forms (one {# letter}).
const ph = s => [...new Set([...String(s).replace(/\b(zero|one|two|few|many|other)\s*\{|=\d+\s*\{/g, '(').matchAll(/\{(\w+)\s*[,}]/g)].map(m => m[1]))].sort().join(',')
for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.json') && !f.endsWith('.tmp') && f !== 'en.json' && f !== 'server-patterns.json')) {
  let d
  try { d = read(path.join(dir, f)) } catch (e) { problem(`${f}: invalid JSON — ${e.message}`); continue }
  const code = f.replace(/\.json$/, '')
  let cats = ['other']
  try { cats = new Intl.PluralRules(code).resolvedOptions().pluralCategories } catch {}
  for (const [k, v] of Object.entries(d)) {
    if (!(k in en)) problem(`${f}: ${k} is not in en.json`)
    else if (ph(v) !== ph(en[k])) problem(`${f}: ${k} placeholders {${ph(v)}} ≠ English {${ph(en[k])}}`)
    // Every plural needs the forms this language distinguishes (Arabic six, Polish four, Japanese one).
    for (const m of String(v).matchAll(/\{\s*\w+\s*,\s*plural\s*,/g)) {
      const body = String(v).slice(m.index)
      // A choice without the number shown ("=1 {a link} other {links}") needs only those forms.
      if (!body.includes('#') && /=\d+\s*\{/.test(body) && /\bother\s*\{/.test(body)) break
      const have = new Set([...body.matchAll(/\b(zero|one|two|few|many|other)\s*\{/g)].map(x => x[1]))
      const lack = cats.filter(c => !have.has(c) && c !== 'zero' && !(c === 'one' && /=1\s*\{/.test(body)))
      if (!have.has('other') || lack.length) problem(`${f}: ${k} plural lacks ${['other', ...lack].filter(c => !have.has(c)).join(', ')}`)
      break
    }
    try { if (typeof v !== 'string' || !v.trim()) problem(`${f}: ${k} is empty`) } catch {}
  }
  const missing = Object.keys(en).filter(k => !(k in d)).length
  if (missing) console.log(`${f}: ${missing} keys not translated (English is shown)`)
}
// Cyrillic outside the locales: interface text that was missed (detection patterns are marked by hand).
for (const f of files) {
  fs.readFileSync(f, 'utf8').split('\n').forEach((l, i) => { if (/[А-Яа-яЁё]/.test(l)) console.log(`cyrillic ${path.relative(root, f)}:${i + 1}: ${l.trim().slice(0, 100)}`) })
}
console.log(problems ? `${problems} problems` : 'ok')
process.exit(problems ? 1 : 0)
