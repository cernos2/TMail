// Splits the English source (with Russian alongside, for meaning) into parts for translators:
// <dir>/part-<n>.json = { key: { en, ru } }. Only keys missing in <code>.json when a code is given.
// Usage: node scripts/i18n-split.mjs <dir> [size=420] [code]
import fs from 'node:fs'
import path from 'node:path'

const [dir, sizeArg, code] = process.argv.slice(2)
const size = Number(sizeArg) || 420
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..')
const read = f => JSON.parse(fs.readFileSync(path.join(root, 'app/locales', f), 'utf8'))
const en = read('en.json'), ru = read('ru.json')
const have = code && fs.existsSync(path.join(root, 'app/locales', `${code}.json`)) ? read(`${code}.json`) : {}
const keys = Object.keys(en).filter(k => !(k in have))
fs.mkdirSync(dir, { recursive: true })
let n = 0
for (let i = 0; i < keys.length; i += size) {
  const part = Object.fromEntries(keys.slice(i, i + size).map(k => [k, { en: en[k], ru: ru[k] ?? '' }]))
  fs.writeFileSync(path.join(dir, `part-${++n}.json`), JSON.stringify(part, null, 1))
}
console.log(`${keys.length} keys → ${n} parts in ${dir}`)
