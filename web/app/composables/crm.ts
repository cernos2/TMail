// Counterparty cards (companies and people), filled from letters by Loggi and editable by hand.
import { reactive } from 'vue'
import { api } from './api'
import { toast } from './toast'
import { t, i18n } from './i18n'
import { mail } from './mail'

export type CrmFile = { account: string, folder: string, uid: number, part: string, name: string, mime: string, size: number, kind: string, sentAt: string, preview: string }
export type CrmRel = { id: number, name: string, own: boolean, role?: 'supplier' | 'buyer' | 'via' | 'forwarded', letters: number, lastLetter?: string }
export type CrmLetter = { account: string, folder: string, uid: number, messageId: string, subject: string, sentAt: string }
export type Party = {
  // inn: the main tax / registration ID of the company in its country; ids: its other identifiers
  // ({"VAT": "DE123456789"}); bik: the bank code; account: the account number or IBAN.
  // kpp, ogrn and corrAccount exist only for Russian companies.
  id: number, kind: 'company' | 'person', name: string, country: string, ids: Record<string, string>, inn: string, kpp: string, ogrn: string, legalAddress: string, bankName: string,
  bik: string, account: string, corrAccount: string, website: string, position: string, notes: string, phones: string[], emails: string[],
  domains: string[], own?: boolean, companyId?: number | null, companyName?: string, lastLetter?: string, letters: number, pending: number,
  addresses?: { id: number, kind: string, address: string }[], contacts?: Party[], files?: CrmFile[], history?: CrmLetter[],
  suggestions?: { id: number, field: string, value: string, source: string, target?: string }[],
  companies?: CrmRel[], links?: CrmRel[],
}

// Names of the fields in suggestions; getters, so they follow a language switch.
export const FIELD_NAMES: Record<string, string> = {
  get name() { return t('common.name') }, get inn() { return t('crm.taxId') }, get country() { return t('crm.country') }, get kpp() { return t('crm.kpp') }, get ogrn() { return t('crm.ogrn') },
  get legal_address() { return t('crm.legalAddress') }, get bank_name() { return t('crm.bank') }, get bik() { return t('crm.bik') },
  get account() { return t('crm.account') }, get corr_account() { return t('crm.corrAccount') }, get website() { return t('crm.website') },
  get position() { return t('crm.position') }, get notes() { return t('crm.notes') },
}

// The country this mail service works in (COUNTRY chosen at installation), '' when not set.
export const serverCountry = () => { const c = mail.me?.defaults?.country || ''; return /^[A-Z]{2}$/.test(c) ? c : '' }

// The usual local name of a company's main tax / registration ID in its country. Proper names of
// the identifiers, not translated; Russia's comes from the locale (ИНН / INN).
const TAX_ID_NAMES: Record<string, string> = {
  KZ: 'BIN', BY: 'UNP', UA: 'EDRPOU', UZ: 'STIR', KG: 'INN', AM: 'TIN', GE: 'ID code', US: 'EIN', CA: 'BN', GB: 'Company number',
  IE: 'CRO number', DE: 'USt-IdNr.', AT: 'UID', CH: 'UID', FR: 'SIREN', BE: 'Enterprise number', LU: 'TVA', IT: 'P.IVA', ES: 'NIF',
  PT: 'NIF', PL: 'NIP', CZ: 'IČO', SK: 'IČO', HU: 'Adószám', RO: 'CUI', BG: 'EIK', NL: 'KvK', DK: 'CVR', SE: 'Org.nr', NO: 'Org.nr',
  FI: 'Y-tunnus', EE: 'Registrikood', LV: 'Reģ. nr.', LT: 'Įmonės kodas', GR: 'AFM', TR: 'VKN', IL: 'Company number', AE: 'TRN',
  SA: 'VAT number', IN: 'GSTIN', CN: 'USCC', HK: 'BR number', JP: 'Corporate number', KR: 'BRN', SG: 'UEN', MY: 'SSM', TH: 'Tax ID',
  ID: 'NPWP', VN: 'MST', PH: 'TIN', AU: 'ABN', NZ: 'NZBN', BR: 'CNPJ', MX: 'RFC', AR: 'CUIT', CL: 'RUT', CO: 'NIT', PE: 'RUC', ZA: 'Reg. number',
}
export function taxIdLabel(country?: string): string {
  const c = (country || '').toUpperCase()
  if (c === 'RU') return t('crm.inn')
  return TAX_ID_NAMES[c] || t('crm.taxId')
}
// The name of the bank code: BIK in Russia, routing number in the USA, sort code in the UK…
const BANK_CODE_NAMES: Record<string, string> = { US: 'Routing number', GB: 'Sort code', IE: 'BIC', IN: 'IFSC', AU: 'BSB', CA: 'Transit number' }
export function bankCodeLabel(country?: string): string {
  const c = (country || '').toUpperCase()
  if (c === 'RU' || c === 'KZ' || c === 'BY') return t('crm.bik')
  return BANK_CODE_NAMES[c] || t('crm.bankCode')
}
// The country of a card for the labels: its own, or the server's when it is not known.
export const cardCountry = (p: Pick<Party, 'country'>) => p.country || serverCountry()
// KPP, OGRN and the correspondent account are Russian: shown for Russian cards (or cards of unknown
// country on a server in Russia) and for any card that has them filled.
export const showRussianFields = (p: Pick<Party, 'country' | 'kpp' | 'ogrn' | 'corrAccount'>) =>
  cardCountry(p) === 'RU' || !!(p.kpp || p.ogrn || p.corrAccount)
// The name of a country in the interface language ("DE" → "Germany").
export function countryName(code: string): string {
  if (!code) return ''
  try { return new Intl.DisplayNames([i18n.lang], { type: 'region' }).of(code) || code } catch { return code }
}
// Every country for a picker, sorted by name in the interface language.
export function countryOptions(): { code: string, name: string }[] {
  const out: { code: string, name: string }[] = []
  let names: Intl.DisplayNames
  try { names = new Intl.DisplayNames([i18n.lang], { type: 'region', fallback: 'none' }) } catch { return out }
  const skip = new Set(['EU', 'EZ', 'UN', 'QO', 'XA', 'XB', 'ZZ', 'AA', 'SU', 'YU', 'DD', 'CS', 'AN', 'BU', 'TP', 'ZR', 'NT', 'FX', 'QU'])
  for (let a = 65; a <= 90; a++) for (let b = 65; b <= 90; b++) {
    const code = String.fromCharCode(a, b)
    const name = skip.has(code) ? undefined : names.of(code)
    if (name && name !== code) out.push({ code, name })
  }
  return out.sort((x, y) => x.name.localeCompare(y.name, i18n.lang))
}

export const crm = reactive({
  open: false, list: [] as Party[], q: '', kind: '' as '' | 'company' | 'person', loading: false,
  current: null as Party | null, currentLoading: false, editing: null as Party | null,
  busy: '', // what is being done with the open card (saving, merging…): the card is veiled
  lookups: {} as Record<string, Party | null | 'loading'>,
})

let listSeq = 0
export async function loadCrmList() {
  const seq = ++listSeq
  crm.loading = true
  try {
    const r = await api<Party[]>('/api/crm/parties', { query: { q: crm.q, kind: crm.kind } })
    if (seq === listSeq) crm.list = r
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { if (seq === listSeq) crm.loading = false }
}

export async function openParty(id: number) {
  crm.open = true
  crm.currentLoading = true
  crm.editing = null
  try { crm.current = await api<Party>(`/api/crm/parties/${id}`) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { crm.currentLoading = false }
}

export const emptyParty = (kind: 'company' | 'person'): Party => ({ id: 0, kind, name: '', country: kind === 'company' ? serverCountry() : '', ids: {}, inn: '', kpp: '', ogrn: '', legalAddress: '', bankName: '', bik: '',
  account: '', corrAccount: '', website: '', position: '', notes: '', phones: [''], emails: [], domains: [], letters: 0, pending: 0 })

export async function saveParty(p: Party) {
  const saved = p.id
    ? await api<Party>(`/api/crm/parties/${p.id}`, { method: 'PUT', body: p })
    : await api<Party>('/api/crm/parties', { method: 'POST', body: p })
  crm.current = saved
  crm.editing = null
  crm.lookups = {}
  loadCrmList()
  return saved
}
export async function deleteParty(id: number) {
  await api(`/api/crm/parties/${id}`, { method: 'DELETE' })
  crm.current = null
  crm.lookups = {}
  loadCrmList()
}
export async function addAddress(id: number, address: string) {
  await api(`/api/crm/parties/${id}/addresses`, { method: 'POST', body: { address } })
  await openParty(id)
}
export async function removeAddress(partyId: number, id: number) {
  await api(`/api/crm/addresses/${id}`, { method: 'DELETE' })
  await openParty(partyId)
}
export async function answerSuggestion(partyId: number, id: number, accept: boolean) {
  const r = await api<{ open?: number }>(`/api/crm/suggestions/${id}`, { method: 'POST', query: { accept: accept ? '1' : '0' } })
  await openParty(r.open || partyId)
  if (r.open && r.open !== partyId) { crm.lookups = {}; loadCrmList() }
}
// The two cards are one company (or one person): everything goes into `into`.
export async function mergeParty(id: number, into: number) {
  crm.current = await api<Party>(`/api/crm/parties/${id}/merge`, { method: 'POST', body: { into } })
  crm.lookups = {}
  loadCrmList()
}

// Card for a sender (cached per address for the session).
export async function lookupSender(email: string) {
  const k = email.toLowerCase()
  if (k in crm.lookups) return
  crm.lookups[k] = 'loading'
  try { crm.lookups[k] = (await api<Party | ''>('/api/crm/lookup', { query: { email: k } })) || null } catch { crm.lookups[k] = null }
}

export async function extractFromLetter(m: { account: string, folder: string, uid: number, from: { email: string } }) {
  const r = await api<{ id: number }>('/api/crm/extract', { method: 'POST', body: { account: m.account, folder: m.folder, uid: m.uid }, timeout: 180000 })
  delete crm.lookups[m.from.email.toLowerCase()]
  return r.id
}
