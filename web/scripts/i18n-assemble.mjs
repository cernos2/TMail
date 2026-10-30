// Assembles a translation made in parts: <dir>/<code>.<n>.json ({ key: text }) → app/locales/<code>.json,
// keeping only keys that exist in en.json, then checks it (placeholders, plural forms, missing keys).
// Usage: node scripts/i18n-assemble.mjs <code> <dir>
import fs from 'node:fs'
import path from 'node:path'
import { execFileSync } from 'node:child_process'

const [code, dir] = process.argv.slice(2)
if (!code || !dir) { console.error('usage: i18n-assemble.mjs <code> <dir>'); process.exit(2) }
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..')
const en = JSON.parse(fs.readFileSync(path.join(root, 'app/locales/en.json'), 'utf8'))
const target = path.join(root, `app/locales/${code}.json`)
const out = fs.existsSync(target) ? JSON.parse(fs.readFileSync(target, 'utf8')) : {}
for (const f of fs.readdirSync(dir).filter(f => f.startsWith(code + '.') && f.endsWith('.json')).sort()) {
  let part
  try { part = JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8')) } catch (e) { console.error(`${f}: invalid JSON — ${e.message}`); process.exit(1) }
  for (const [k, v] of Object.entries(part)) if (k in en && typeof v === 'string' && v.trim()) out[k] = v
}
const sorted = Object.fromEntries(Object.keys(en).filter(k => k in out).map(k => [k, out[k]]))
fs.writeFileSync(target + '.tmp', JSON.stringify(sorted, null, 1) + '\n')
fs.renameSync(target + '.tmp', target) // atomic: others check every file meanwhile
const missing = Object.keys(en).filter(k => !(k in sorted))
console.log(`${code}.json: ${Object.keys(sorted).length} of ${Object.keys(en).length} keys${missing.length ? `; missing: ${missing.slice(0, 30).join(', ')}${missing.length > 30 ? ' …' : ''}` : ''}`)
try {
  const r = execFileSync('node', [path.join(root, 'scripts/i18n-check.mjs')], { encoding: 'utf8' })
  void r
} catch (e) {
  const lines = String(e.stdout || '').split('\n').filter(l => l.includes(`${code}.json:`) && !l.includes('keys not translated'))
  if (lines.length) { console.log(lines.join('\n')); process.exit(1) }
}
console.log('checks passed')
