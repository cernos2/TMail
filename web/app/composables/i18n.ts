// Interface translations. Messages live in app/locales/<code>.json as flat "area.key" → text;
// English is the base every other language falls back to. A message may hold {name} placeholders
// and ICU-style plurals: "{n, plural, one {# letter} other {# letters}}" (the plural forms of each
// language come from Intl.PluralRules). Dates and times are shown in the user's time zone.
import { reactive } from 'vue'
import en from '~/locales/en.json'

export type Lang = { code: string, name: string, en: string, rtl?: boolean }

// The 100 most spoken languages; name is the language's own name for itself.
export const LANGS: Lang[] = [
  { code: 'en', name: 'English', en: 'English' },
  { code: 'zh-CN', name: '简体中文', en: 'Chinese (Simplified)' },
  { code: 'zh-TW', name: '繁體中文', en: 'Chinese (Traditional)' },
  { code: 'hi', name: 'हिन्दी', en: 'Hindi' },
  { code: 'es', name: 'Español', en: 'Spanish' },
  { code: 'fr', name: 'Français', en: 'French' },
  { code: 'ar', name: 'العربية', en: 'Arabic', rtl: true },
  { code: 'bn', name: 'বাংলা', en: 'Bengali' },
  { code: 'pt-BR', name: 'Português (Brasil)', en: 'Portuguese (Brazil)' },
  { code: 'pt-PT', name: 'Português (Portugal)', en: 'Portuguese (Portugal)' },
  { code: 'ru', name: 'Русский', en: 'Russian' },
  { code: 'ur', name: 'اردو', en: 'Urdu', rtl: true },
  { code: 'id', name: 'Bahasa Indonesia', en: 'Indonesian' },
  { code: 'de', name: 'Deutsch', en: 'German' },
  { code: 'ja', name: '日本語', en: 'Japanese' },
  { code: 'mr', name: 'मराठी', en: 'Marathi' },
  { code: 'te', name: 'తెలుగు', en: 'Telugu' },
  { code: 'tr', name: 'Türkçe', en: 'Turkish' },
  { code: 'ta', name: 'தமிழ்', en: 'Tamil' },
  { code: 'vi', name: 'Tiếng Việt', en: 'Vietnamese' },
  { code: 'fil', name: 'Filipino', en: 'Filipino' },
  { code: 'ko', name: '한국어', en: 'Korean' },
  { code: 'fa', name: 'فارسی', en: 'Persian', rtl: true },
  { code: 'ha', name: 'Hausa', en: 'Hausa' },
  { code: 'sw', name: 'Kiswahili', en: 'Swahili' },
  { code: 'jv', name: 'Basa Jawa', en: 'Javanese' },
  { code: 'it', name: 'Italiano', en: 'Italian' },
  { code: 'pa', name: 'ਪੰਜਾਬੀ', en: 'Punjabi' },
  { code: 'gu', name: 'ગુજરાતી', en: 'Gujarati' },
  { code: 'th', name: 'ไทย', en: 'Thai' },
  { code: 'kn', name: 'ಕನ್ನಡ', en: 'Kannada' },
  { code: 'am', name: 'አማርኛ', en: 'Amharic' },
  { code: 'yo', name: 'Yorùbá', en: 'Yoruba' },
  { code: 'pl', name: 'Polski', en: 'Polish' },
  { code: 'ml', name: 'മലയാളം', en: 'Malayalam' },
  { code: 'or', name: 'ଓଡ଼ିଆ', en: 'Odia' },
  { code: 'my', name: 'မြန်မာ', en: 'Burmese' },
  { code: 'uk', name: 'Українська', en: 'Ukrainian' },
  { code: 'su', name: 'Basa Sunda', en: 'Sundanese' },
  { code: 'ig', name: 'Igbo', en: 'Igbo' },
  { code: 'uz', name: 'Oʻzbekcha', en: 'Uzbek' },
  { code: 'sd', name: 'سنڌي', en: 'Sindhi', rtl: true },
  { code: 'ro', name: 'Română', en: 'Romanian' },
  { code: 'nl', name: 'Nederlands', en: 'Dutch' },
  { code: 'az', name: 'Azərbaycanca', en: 'Azerbaijani' },
  { code: 'ps', name: 'پښتو', en: 'Pashto', rtl: true },
  { code: 'ne', name: 'नेपाली', en: 'Nepali' },
  { code: 'si', name: 'සිංහල', en: 'Sinhala' },
  { code: 'km', name: 'ខ្មែរ', en: 'Khmer' },
  { code: 'zu', name: 'isiZulu', en: 'Zulu' },
  { code: 'so', name: 'Soomaali', en: 'Somali' },
  { code: 'mg', name: 'Malagasy', en: 'Malagasy' },
  { code: 'kk', name: 'Қазақша', en: 'Kazakh' },
  { code: 'hu', name: 'Magyar', en: 'Hungarian' },
  { code: 'el', name: 'Ελληνικά', en: 'Greek' },
  { code: 'cs', name: 'Čeština', en: 'Czech' },
  { code: 'sv', name: 'Svenska', en: 'Swedish' },
  { code: 'be', name: 'Беларуская', en: 'Belarusian' },
  { code: 'ht', name: 'Kreyòl ayisyen', en: 'Haitian Creole' },
  { code: 'he', name: 'עברית', en: 'Hebrew', rtl: true },
  { code: 'rw', name: 'Kinyarwanda', en: 'Kinyarwanda' },
  { code: 'sr', name: 'Српски', en: 'Serbian' },
  { code: 'bg', name: 'Български', en: 'Bulgarian' },
  { code: 'hr', name: 'Hrvatski', en: 'Croatian' },
  { code: 'sk', name: 'Slovenčina', en: 'Slovak' },
  { code: 'da', name: 'Dansk', en: 'Danish' },
  { code: 'fi', name: 'Suomi', en: 'Finnish' },
  { code: 'nb', name: 'Norsk bokmål', en: 'Norwegian' },
  { code: 'xh', name: 'isiXhosa', en: 'Xhosa' },
  { code: 'ky', name: 'Кыргызча', en: 'Kyrgyz' },
  { code: 'tg', name: 'Тоҷикӣ', en: 'Tajik' },
  { code: 'tk', name: 'Türkmençe', en: 'Turkmen' },
  { code: 'ka', name: 'ქართული', en: 'Georgian' },
  { code: 'hy', name: 'Հայերեն', en: 'Armenian' },
  { code: 'lt', name: 'Lietuvių', en: 'Lithuanian' },
  { code: 'lv', name: 'Latviešu', en: 'Latvian' },
  { code: 'et', name: 'Eesti', en: 'Estonian' },
  { code: 'sl', name: 'Slovenščina', en: 'Slovenian' },
  { code: 'sq', name: 'Shqip', en: 'Albanian' },
  { code: 'mk', name: 'Македонски', en: 'Macedonian' },
  { code: 'bs', name: 'Bosanski', en: 'Bosnian' },
  { code: 'mn', name: 'Монгол', en: 'Mongolian' },
  { code: 'lo', name: 'ລາວ', en: 'Lao' },
  { code: 'ku', name: 'Kurdî', en: 'Kurdish (Kurmanji)' },
  { code: 'ckb', name: 'کوردی', en: 'Kurdish (Sorani)', rtl: true },
  { code: 'tt', name: 'Татарча', en: 'Tatar' },
  { code: 'ca', name: 'Català', en: 'Catalan' },
  { code: 'gl', name: 'Galego', en: 'Galician' },
  { code: 'eu', name: 'Euskara', en: 'Basque' },
  { code: 'ga', name: 'Gaeilge', en: 'Irish' },
  { code: 'cy', name: 'Cymraeg', en: 'Welsh' },
  { code: 'is', name: 'Íslenska', en: 'Icelandic' },
  { code: 'af', name: 'Afrikaans', en: 'Afrikaans' },
  { code: 'ms', name: 'Bahasa Melayu', en: 'Malay' },
  { code: 'as', name: 'অসমীয়া', en: 'Assamese' },
  { code: 'om', name: 'Afaan Oromoo', en: 'Oromo' },
  { code: 'ti', name: 'ትግርኛ', en: 'Tigrinya' },
  { code: 'ceb', name: 'Cebuano', en: 'Cebuano' },
  { code: 'ny', name: 'Chichewa', en: 'Chichewa' },
  { code: 'sn', name: 'chiShona', en: 'Shona' },
]

type Dict = Record<string, string>
const loaders = import.meta.glob<{ default: Dict }>('../locales/*.json')
const fallback: Dict = en
let dict: Dict = en

// version changes on every switch: t() reads it, so everything on screen is re-rendered.
export const i18n = reactive({ lang: 'en', tz: '', version: 0 })

export const langInfo = (code: string) => LANGS.find(l => l.code === code)
// The closest language we have: "pt" → "pt-BR", "zh-Hans-CN" → "zh-CN", "en-GB" → "en".
export function matchLang(code: string | undefined | null): string | null {
  if (!code) return null
  if (langInfo(code)) return code
  const base = code.toLowerCase().split(/[-_]/)[0]!
  if (base === 'zh') return /tw|hk|mo|hant/i.test(code) ? 'zh-TW' : 'zh-CN'
  if (base === 'pt') return /pt[-_]pt/i.test(code) ? 'pt-PT' : 'pt-BR'
  if (base === 'no' || base === 'nn') return 'nb'
  if (base === 'tl') return 'fil'
  return LANGS.find(l => l.code.toLowerCase().split('-')[0] === base)?.code || null
}
export const browserLang = () => (typeof navigator !== 'undefined' ? (navigator.languages || [navigator.language]).map(matchLang).find(Boolean) : null) || 'en'
export const browserTz = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' } catch { return 'UTC' } }

export async function setLang(code: string) {
  const lang = matchLang(code) || 'en'
  const load = loaders[`../locales/${lang}.json`]
  try { dict = load ? (await load()).default : fallback } catch { dict = fallback }
  i18n.lang = lang
  if (typeof document !== 'undefined') {
    document.documentElement.lang = lang
    document.documentElement.dir = langInfo(lang)?.rtl ? 'rtl' : 'ltr'
  }
  try { localStorage.setItem('zm-lang', lang) } catch {}
  plurals.clear(); fmts.clear()
  i18n.version++
}
export function setTimezone(tz: string) {
  i18n.tz = tz || browserTz()
  try { localStorage.setItem('zm-tz', tz) } catch {}
  fmts.clear()
  i18n.version++
}
export const savedLang = () => { try { return localStorage.getItem('zm-lang') } catch { return null } }
export const savedTz = () => { try { return localStorage.getItem('zm-tz') || '' } catch { return '' } }

// ---------- messages ----------

const plurals = new Map<string, Intl.PluralRules>()
function pluralCat(n: number) {
  let r = plurals.get(i18n.lang)
  if (!r) { try { r = new Intl.PluralRules(i18n.lang) } catch { r = new Intl.PluralRules('en') } plurals.set(i18n.lang, r) }
  return r.select(n)
}

// The block between braces starting at s[i] === '{', or -1.
function closing(s: string, i: number) {
  let depth = 0
  for (let j = i; j < s.length; j++) {
    if (s[j] === '{') depth++
    else if (s[j] === '}' && --depth === 0) return j
  }
  return -1
}

function format(msg: string, p: Record<string, any>): string {
  let out = ''
  for (let i = 0; i < msg.length; i++) {
    if (msg[i] !== '{') { out += msg[i]; continue }
    const end = closing(msg, i)
    if (end < 0) { out += msg.slice(i); break }
    const body = msg.slice(i + 1, end)
    const m = /^\s*(\w+)\s*,\s*plural\s*,(.*)$/s.exec(body)
    if (m) {
      const n = Number(p[m[1]!]) || 0
      const forms: Record<string, string> = {}
      const rest = m[2]!
      for (let k = 0; k < rest.length;) {
        const sel = /^\s*(=\d+|zero|one|two|few|many|other)\s*\{/.exec(rest.slice(k))
        if (!sel) break
        const open = k + sel[0].length - 1
        const close = closing(rest, open)
        if (close < 0) break
        forms[sel[1]!] = rest.slice(open + 1, close)
        k = close + 1
      }
      const form = forms[`=${n}`] ?? forms[pluralCat(n)] ?? forms.other ?? ''
      out += format(form.replace(/#/g, num(n)), p)
    } else {
      const v = p[body.trim()]
      out += v === undefined ? `{${body}}` : String(v)
    }
    i = end
  }
  return out
}

export function t(key: string, p?: Record<string, any>): string {
  void i18n.version // re-render on a language switch
  const msg = dict[key] ?? fallback[key] ?? key
  return p ? format(msg, p) : msg
}

// ---------- numbers, dates, times (in the user's time zone) ----------

const fmts = new Map<string, Intl.DateTimeFormat | Intl.NumberFormat>()
// 12-hour languages write "4:36 PM", not "04:36 PM": a two-digit hour is only for 24-hour clocks.
const twelveHour = () => { try { return /h1[12]/.test(new Intl.DateTimeFormat(i18n.lang, { hour: 'numeric' }).resolvedOptions().hourCycle || '') } catch { return false } }
function dtf(opts: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  if (opts.hour === '2-digit' && twelveHour()) opts = { ...opts, hour: 'numeric' }
  const key = 'd' + JSON.stringify(opts)
  let f = fmts.get(key) as Intl.DateTimeFormat | undefined
  if (!f) {
    try { f = new Intl.DateTimeFormat(i18n.lang, { timeZone: i18n.tz || undefined, ...opts }) }
    catch { f = new Intl.DateTimeFormat('en', opts) }
    fmts.set(key, f)
  }
  return f
}
export function num(n: number, opts?: Intl.NumberFormatOptions): string {
  void i18n.version
  const key = 'n' + JSON.stringify(opts || {})
  let f = fmts.get(key) as Intl.NumberFormat | undefined
  if (!f) { f = new Intl.NumberFormat(i18n.lang, opts); fmts.set(key, f) }
  return f.format(n)
}
export function fmtDate(d: Date | string | number, opts: Intl.DateTimeFormatOptions): string {
  void i18n.version
  const x = d instanceof Date ? d : new Date(d)
  return isNaN(x.getTime()) ? '' : dtf(opts).format(x)
}
// The calendar day of a moment in the user's time zone: "2026-10-29".
export function tzDayKey(d: Date | string | number): string {
  const x = d instanceof Date ? d : new Date(d)
  const parts = dtf({ year: 'numeric', month: '2-digit', day: '2-digit', calendar: 'gregory', numberingSystem: 'latn' } as Intl.DateTimeFormatOptions).formatToParts(x)
  const get = (t: string) => parts.find(p => p.type === t)?.value || ''
  return `${get('year')}-${get('month')}-${get('day')}`
}
// Month and weekday names for pickers and the calendar.
export function monthNames(style: 'long' | 'short' = 'long'): string[] {
  return Array.from({ length: 12 }, (_, m) => fmtDate(new Date(Date.UTC(2024, m, 15, 12)), { month: style, timeZone: 'UTC' }))
}
export function weekdayNames(style: 'long' | 'short' | 'narrow' = 'short'): string[] { // Monday first
  // Capitalised like a column title ("mon" → "Mon"); languages without letter case are unchanged.
  return Array.from({ length: 7 }, (_, i) => fmtDate(new Date(Date.UTC(2024, 0, 1 + i, 12)), { weekday: style, timeZone: 'UTC' }))
    .map(w => w.charAt(0).toLocaleUpperCase(i18n.lang) + w.slice(1))
}

// The user's own language and time zone from /api/me; without one, the server's default language
// and the time zone of this device.
export async function applyUserLocale(me: { lang?: string, timezone?: string, defaults?: { lang?: string } } | null) {
  if (!me) return
  const lang = matchLang(me.lang) || matchLang(me.defaults?.lang) || browserLang()
  if (lang !== i18n.lang) await setLang(lang)
  if ((me.timezone || browserTz()) !== i18n.tz) setTimezone(me.timezone || '')
}
