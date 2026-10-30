// Exports the texts of the pages the Go server renders itself ("filepage.*" keys) from every
// language in app/locales into backend/cmd/zmaild/pagelocale.json (embedded into the server).
// Run after changing those texts or adding a language: node scripts/export-server-i18n.mjs
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..')
const dir = path.join(root, 'app/locales')
const out = {}
for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.json') && f !== 'server-patterns.json').sort()) {
  const d = JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8'))
  const keys = Object.keys(d).filter(k => k.startsWith('filepage.'))
  if (keys.length) out[f.replace(/\.json$/, '')] = Object.fromEntries(keys.map(k => [k, d[k]]))
}
const target = path.join(root, '../backend/cmd/zmaild/pagelocale.json')
fs.writeFileSync(target, JSON.stringify(out, null, 1) + '\n')
console.log(`${Object.keys(out).length} languages → ${path.relative(path.join(root, '..'), target)}`)
