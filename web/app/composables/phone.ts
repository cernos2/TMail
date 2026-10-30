// Phone numbers of any country, through libphonenumber: a number written without a country code is
// read as a number of the server's country (COUNTRY chosen at installation). Numbers with the code +7
// (Russia, Kazakhstan) keep the house style +7(XXX) XXX-XX-XX; all others are shown in the
// international format (+49 30 1234567). An extension is kept after the number.
import { AsYouType, parsePhoneNumberFromString, type CountryCode } from 'libphonenumber-js/min'
import { mail } from './mail'

// Phone data format, not interface text: "доб." (Russian for "ext.") is recognised next to "ext."
// and "#", and extensions of +7 numbers are stored with it, as they always were.
const EXT_RE = /\s*(доб\.?|ext\.?|x|#)\s*(\d{1,6})\s*$/i
const EXT_RU = 'доб.'

const country = (): CountryCode | undefined => {
  const c = mail.me?.defaults?.country
  return c && /^[A-Z]{2}$/.test(c) ? c as CountryCode : undefined
}

function ru(d: string) { // ten digits after +7
  return `+7(${d.slice(0, 3)}) ${d.slice(3, 6)}-${d.slice(6, 8)}-${d.slice(8)}`
}

export function formatPhone(raw: string): string {
  const s = raw.trim()
  const ext = s.match(EXT_RE)
  const main = ext ? s.slice(0, ext.index) : s
  const p = parsePhoneNumberFromString(main, country())
  if (!p || !p.isPossible()) return s
  if (p.countryCallingCode === '7' && p.nationalNumber.length === 10) return ru(p.nationalNumber) + (ext ? ` ${EXT_RU} ${ext[2]}` : '')
  return p.formatInternational() + (ext ? ` ext. ${ext[2]}` : '')
}

// Formatting while typing.
export function maskPhone(raw: string): string {
  if (EXT_RE.test(raw) || /доб|ext/i.test(raw)) return raw
  const c = country()
  let d = raw.replace(/\D/g, '')
  if (!d) return raw.trim() === '+' ? raw : ''
  // +7 numbers: the familiar mask, also for 8XXXXXXXXXX dialled the Russian way.
  const plus = raw.trim().startsWith('+')
  if ((plus && d.startsWith('7')) || (!plus && (c === 'RU' || c === 'KZ') && /^[78]/.test(d) && d.length <= 11)) {
    d = d.slice(1, 11)
    let out = '+7(' + d.slice(0, 3)
    if (d.length >= 3) out += ') ' + d.slice(3, 6)
    if (d.length >= 6) out += '-' + d.slice(6, 8)
    if (d.length >= 8) out += '-' + d.slice(8, 10)
    return out
  }
  return new AsYouType(plus ? undefined : c).input(plus ? '+' + d : d)
}
