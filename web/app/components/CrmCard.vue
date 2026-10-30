<script setup lang="ts">
// One counterparty card: requisites, contacts, delivery addresses, people, files, letters.
import { computed, ref, watch } from 'vue'
import { mail, openMessage, type Summary } from '~/composables/mail'
import { api } from '~/composables/api'
import { crm, loadCrmList, openParty, saveParty, deleteParty, addAddress, removeAddress, answerSuggestion, mergeParty, FIELD_NAMES, type Party, type CrmFile, type CrmRel,
  taxIdLabel, bankCodeLabel, cardCountry, showRussianFields, countryName, countryOptions } from '~/composables/crm'
import { ext, extColor, openAttachment } from '~/composables/attachments'
import { fileSize, longDate } from '~/composables/format'
import { confirmDialog } from '~/composables/dialogs'
import { formatPhone, maskPhone } from '~/composables/phone'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'
import { aiName } from '~/composables/ai'

const p = computed(() => crm.current)
const e = computed(() => crm.editing)
const newAddr = ref('')
const busy = ref(false)
const err = ref('')
const kindFile = (k: string) => k === 'requisites' ? t('crm.fileRequisites') : k === 'contract' ? t('crm.fileContract') : k === 'invoice' ? t('crm.fileInvoice') : ''

// Company details; the names of the identifiers follow the company's country. copy: the value has
// its own copy button (identifiers and account numbers).
const req = computed(() => {
  const x = p.value
  if (!x || x.kind !== 'company') return []
  const cc = cardCountry(x)
  const ru = cc === 'RU'
  const rows: [string, string, boolean][] = [[t('crm.country'), countryName(x.country), false], [taxIdLabel(cc), x.inn, true], [t('crm.kpp'), x.kpp, true], [t('crm.ogrn'), x.ogrn, true],
    ...Object.entries(x.ids || {}).map(([k, v]) => [k, v, true] as [string, string, boolean]),
    [t('crm.legalAddressShort'), x.legalAddress, false], [t('crm.bank'), x.bankName, false], [bankCodeLabel(cc), x.bik, true],
    [ru ? t('crm.accountShort') : t('crm.accountIban'), x.account, true], [t('crm.corrAccountShort'), x.corrAccount, true]]
  return rows.filter(r => r[1])
})
const reqText = computed(() => p.value ? [p.value.name, ...req.value.map(([k, v]) => `${k}: ${v}`)].join('\n') : '')
const countries = computed(() => countryOptions())
// The other identifiers of the card being edited, as rows (label, value).
const idRows = ref<{ k: string, v: string }[]>([])
watch(() => crm.editing, x => { idRows.value = Object.entries(x?.ids || {}).map(([k, v]) => ({ k, v })) }, { immediate: true })

function startEdit() { if (p.value) { const x = JSON.parse(JSON.stringify(p.value)); if (!x.phones.length) x.phones = ['']; crm.editing = x } }
function listText(a: string[]) { return a.join('\n') }
async function save() {
  const x = e.value!
  x.phones = x.phones.map(formatPhone).filter(Boolean)
  x.ids = Object.fromEntries(idRows.value.map(r => [r.k.trim(), r.v.trim()]).filter(([k, v]) => k && v))
  if (!x.name.trim()) { err.value = x.kind === 'company' ? t('crm.enterName') : t('crm.enterFullName'); return }
  busy.value = true; err.value = ''; crm.busy = t('crm.savingCard')
  try { await saveParty(x); toast(t('crm.cardSaved')) } catch (ex: any) { err.value = ex.message } finally { busy.value = false; crm.busy = '' }
}
// "It's us": our own company or colleague — the card stays with its links, marked as ours.
async function markSelf() {
  if (!p.value) return
  const company = p.value.kind === 'company'
  if (!await confirmDialog(company ? t('crm.ownCompanyConfirm', { name: p.value.name }) : t('crm.ownPersonConfirm', { name: p.value.name }),
    company ? t('crm.ownCompanyText') : t('crm.ownPersonText'), { ok: t('crm.yesItsUs') })) return
  crm.busy = t('common.saving')
  try {
    await api(`/api/crm/parties/${p.value.id}/self`, { method: 'POST' })
    toast(t('common.done'))
    crm.lookups = {}; loadCrmList(); openParty(p.value.id)
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { crm.busy = '' }
}

// Links: companies of a person; suppliers / buyers / documents passed on for a company.
const roleName = (r: 'supplier' | 'buyer' | 'via' | 'forwarded') => r === 'supplier' ? t('crm.roleSupplier') : r === 'buyer' ? t('crm.roleBuyer') : r === 'via' ? t('crm.roleVia') : t('crm.roleForwarded')
const linkGroups = computed(() => {
  const g: Record<string, CrmRel[]> = {}
  for (const l of p.value?.links || []) (g[l.role || ''] ||= []).push(l)
  return (['supplier', 'buyer', 'via', 'forwarded'] as const).filter(r => g[r]?.length).map(r => ({ role: r, title: roleName(r), items: g[r]! }))
})
const relMeta = (r: CrmRel) => [r.letters ? t('crm.letters', { n: r.letters }) : '', r.lastLetter ? t('crm.lastLetter', { date: longDate(r.lastLetter) }) : ''].filter(Boolean).join(' · ')

// Merge with another card: the same company written differently, or one person under two cards.
const merging = ref(false)
const mergeQ = ref('')
const mergeList = ref<Party[]>([])
let mergeT: ReturnType<typeof setTimeout> | undefined
function mergeSearch() {
  clearTimeout(mergeT)
  mergeT = setTimeout(async () => {
    if (!p.value) return
    try {
      const r = await api<Party[]>('/api/crm/parties', { query: { q: mergeQ.value.trim(), kind: p.value.kind } })
      mergeList.value = r.filter(x => x.id !== p.value!.id).slice(0, 12)
    } catch {}
  }, 250)
}
function startMerge() { merging.value = true; mergeQ.value = ''; mergeList.value = []; mergeSearch() }
async function doMerge(target: Party) {
  if (!p.value) return
  if (!await confirmDialog(t('crm.mergeConfirm', { name: p.value.name, target: target.name }),
    t('crm.mergeText', { name: p.value.name, target: target.name }), { ok: t('crm.merge') })) return
  crm.busy = t('crm.mergingCards')
  try { await mergeParty(p.value.id, target.id); merging.value = false; toast(t('crm.cardsMerged')) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { crm.busy = '' }
}
async function remove() {
  if (!p.value || !await confirmDialog(t('crm.deleteConfirm', { name: p.value.name }), t('crm.deleteText'), { ok: t('common.delete'), danger: true })) return
  crm.busy = t('crm.deletingCard')
  try { await deleteParty(p.value.id) } finally { crm.busy = '' }
}
async function copy(text: string) { try { await navigator.clipboard.writeText(text); toast(t('common.copied')) } catch {} }
function openFile(f: CrmFile) {
  openAttachment({ account: f.account, folder: f.folder, uid: f.uid },
    { part: f.part, name: f.name, mime: f.mime, size: f.size, inline: false, preview: f.preview as any })
}
function openLetter(l: { account: string, folder: string, uid: number }) {
  openMessage({ account: l.account, folder: l.folder, uid: l.uid, seen: true } as Summary)
}
async function addAddr() {
  if (!p.value || newAddr.value.trim().length < 5) return
  crm.busy = t('crm.addingAddress')
  try { await addAddress(p.value.id, newAddr.value.trim()); newAddr.value = '' } catch (ex: any) { toast(ex.message, { kind: 'err' }) } finally { crm.busy = '' }
}
function back() { crm.current = null; crm.editing = null }
// The suggestion "same company as X?" around the link to X: the text before and after it.
const sameText = () => { const [a = '', b = ''] = t('crm.sameCompany', { target: '\u0000' }).split('\u0000'); return [a, b] }
</script>

<template>
  <!-- edit / create -->
  <div v-if="e" class="crm-body">
    <header class="crm-card-head">
      <button class="icon-btn" :aria-label="t('common.back')" @click="e.id ? (crm.editing = null) : back()"><AppIcon name="back" /></button>
      <h2>{{ e.id ? t('crm.editCard') : e.kind === 'company' ? t('crm.newCompany') : t('crm.newPerson') }}</h2>
    </header>
    <form class="form crm-form" @submit.prevent="save">
      <label class="f">{{ e.kind === 'company' ? t('crm.companyNameLabel') : t('crm.fullName') }}<input v-model="e.name" type="text" /></label>
      <template v-if="e.kind === 'company'">
        <div class="row2">
          <label class="f">{{ t('crm.country') }}<select v-model="e.country"><option value="">{{ t('crm.countryUnknown') }}</option><option v-for="c in countries" :key="c.code" :value="c.code">{{ c.name }}</option></select></label>
          <label class="f">{{ taxIdLabel(cardCountry(e)) }}<input v-model="e.inn" type="text" :inputmode="cardCountry(e) === 'RU' ? 'numeric' : 'text'" /></label>
        </div>
        <div v-if="showRussianFields(e)" class="row2"><label class="f">{{ t('crm.kpp') }}<input v-model="e.kpp" type="text" /></label><label class="f">{{ t('crm.ogrn') }}<input v-model="e.ogrn" type="text" /></label></div>
        <div class="f phones-edit"><span>{{ t('crm.otherIds') }}</span>
          <div v-for="(r, i) in idRows" :key="i" class="phone-row">
            <input v-model="r.k" type="text" :placeholder="t('crm.idLabelPlaceholder')" :aria-label="t('crm.idLabel')" style="flex:0 0 38%" />
            <input v-model="r.v" type="text" :aria-label="t('crm.idValue')" />
            <button type="button" class="icon-btn" :aria-label="t('crm.remove')" @click="idRows.splice(i, 1)"><AppIcon name="close" /></button>
          </div>
          <button type="button" class="link" style="align-self:flex-start" @click="idRows.push({ k: '', v: '' })">{{ t('crm.addId') }}</button>
        </div>
        <label class="f">{{ t('crm.legalAddress') }}<input v-model="e.legalAddress" type="text" /></label>
        <div class="row2"><label class="f">{{ t('crm.bank') }}<input v-model="e.bankName" type="text" /></label><label class="f">{{ bankCodeLabel(cardCountry(e)) }}<input v-model="e.bik" type="text" /></label></div>
        <div class="row2"><label class="f">{{ cardCountry(e) === 'RU' ? t('crm.account') : t('crm.accountIban') }}<input v-model="e.account" type="text" /></label><label v-if="showRussianFields(e)" class="f">{{ t('crm.corrAccount') }}<input v-model="e.corrAccount" type="text" /></label></div>
        <label class="f">{{ t('crm.website') }}<input v-model="e.website" type="text" /></label>
      </template>
      <template v-else>
        <label class="f">{{ t('crm.position') }}<input v-model="e.position" type="text" /></label>
      </template>
      <div class="row2">
        <div class="f phones-edit"><span>{{ t('crm.phones') }}</span>
          <div v-for="(_, i) in e.phones" :key="i" class="phone-row">
            <input v-model="e.phones[i]" type="tel" inputmode="tel" placeholder="+7(___) ___-__-__" @input="e.phones[i] = maskPhone(e.phones[i])" @blur="e.phones[i] = formatPhone(e.phones[i])" />
            <button type="button" class="icon-btn" :aria-label="t('crm.remove')" @click="e.phones.splice(i, 1)"><AppIcon name="close" /></button>
          </div>
          <button type="button" class="link" style="align-self:flex-start" @click="e.phones.push('')">{{ t('crm.addPhone') }}</button>
        </div>
        <label class="f">{{ t('crm.emailsLabel') }}<textarea :value="listText(e.emails)" rows="3" @input="e.emails = ($event.target as HTMLTextAreaElement).value.split('\n')"></textarea></label>
      </div>
      <label v-if="e.kind === 'company'" class="f">{{ t('crm.domainsLabel') }}<input :value="e.domains.join(', ')" type="text" placeholder="firma.ru" @input="e.domains = ($event.target as HTMLInputElement).value.split(/[,\s]+/)" />
        <span class="help">{{ t('crm.domainsHelp') }}</span></label>
      <label class="f">{{ t('crm.notes') }}<textarea v-model="e.notes" rows="3"></textarea></label>
      <p v-if="err" class="err">{{ err }}</p>
      <div class="crm-actions">
        <button v-if="e.id && mail.me?.isAdmin" type="button" class="btn danger-text" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
        <span class="grow"></span>
        <button type="button" class="btn" @click="e.id ? (crm.editing = null) : back()">{{ t('common.cancel') }}</button>
        <button class="btn primary" :disabled="busy">{{ busy ? t('common.saving') : t('common.save') }}</button>
      </div>
    </form>
  </div>

  <!-- view -->
  <div v-else-if="p" class="crm-body">
    <header class="crm-card-head">
      <button class="icon-btn crm-back" :aria-label="t('crm.backToList')" @click="back"><AppIcon name="back" /></button>
      <span class="crm-ico big" :class="p.kind"><AppIcon :name="p.kind === 'company' ? 'home' : 'user'" /></span>
      <div class="crm-title">
        <h2>{{ p.name }}</h2>
        <span class="hint"><span v-if="p.own" class="crm-own">{{ p.kind === 'company' ? t('crm.ownCompany') : t('crm.ownPerson') }}</span>
          <template v-else>{{ p.kind === 'company' ? t('crm.company') : p.companyId ? t('crm.contactPerson') : t('crm.individual') }}</template><template v-if="p.position"> · {{ p.position }}</template>
          <template v-if="p.companyId && (p.companies?.length || 0) <= 1"> · <button class="link" @click="openParty(p.companyId!)">{{ p.companyName }}</button></template>
          <template v-else-if="(p.companies?.length || 0) > 1"> · {{ t('crm.companiesCount', { n: p.companies!.length }) }}</template>
          · {{ t('crm.lettersTotal', { n: p.letters }) }}</span>
      </div>
      <button v-if="mail.me?.isAdmin && !p.own" class="btn sm" :title="p.kind === 'company' ? t('crm.markOwnCompanyTitle') : t('crm.markOwnPersonTitle')" @click="markSelf">{{ t('crm.itsUs') }}</button>
      <button class="btn sm" :title="p.kind === 'company' ? t('crm.mergeCompanyTitle') : t('crm.mergePersonTitle')" @click="startMerge">{{ t('crm.mergeMenu') }}</button>
      <button class="btn sm" @click="startEdit"><AppIcon name="edit" />{{ t('common.edit') }}</button>
    </header>

    <div v-if="p.suggestions?.length" class="crm-sugg">
      <b>{{ t('crm.suggestionsTitle', { ai: aiName() }) }}</b>
      <div v-for="g in p.suggestions" :key="g.id" class="crm-sugg-row">
        <template v-if="g.field === 'same'">
          <span>{{ sameText()[0] }}<button class="link" @click="openParty(+g.value)">{{ g.target }}</button>{{ sameText()[1] }}
            <small v-if="g.source" class="hint">· {{ t('crm.fromLetter', { subject: g.source }) }}</small></span>
          <button class="btn sm primary" @click="answerSuggestion(p.id, g.id, true)">{{ t('crm.yesMerge') }}</button>
          <button class="btn sm" @click="answerSuggestion(p.id, g.id, false)">{{ t('crm.noDifferent') }}</button>
        </template>
        <template v-else>
          <span><i>{{ FIELD_NAMES[g.field] || g.field }}:</i> {{ g.value }} <small v-if="g.source" class="hint">· {{ t('crm.fromLetter', { subject: g.source }) }}</small></span>
          <button class="btn sm primary" @click="answerSuggestion(p.id, g.id, true)">{{ t('crm.replace') }}</button>
          <button class="btn sm" @click="answerSuggestion(p.id, g.id, false)">{{ t('crm.notRight') }}</button>
        </template>
      </div>
    </div>

    <section v-if="merging" class="crm-sec crm-merge">
      <h3>{{ t('crm.mergeWith') }} <button class="link" @click="merging = false">{{ t('crm.cancelLower') }}</button></h3>
      <input v-model="mergeQ" type="search" :placeholder="p.kind === 'company' ? t('crm.mergeSearchCompany') : t('crm.mergeSearchPerson')" autofocus @input="mergeSearch" />
      <button v-for="m in mergeList" :key="m.id" type="button" class="crm-contact" @click="doMerge(m)">
        <span class="crm-ico" :class="m.kind"><AppIcon :name="m.kind === 'company' ? 'home' : 'user'" /></span>
        <span class="crm-txt"><b>{{ m.name }}<span v-if="m.own" class="crm-own">{{ t('crm.ownTag') }}</span></b><span>{{ [m.inn && `${taxIdLabel(cardCountry(m))} ${m.inn}`, m.domains[0] || m.emails[0], m.letters ? t('crm.letters', { n: m.letters }) : ''].filter(Boolean).join(' · ') }}</span></span>
      </button>
      <p v-if="!mergeList.length" class="hint">{{ t('crm.nothingFound') }}</p>
    </section>

    <section v-if="p.companies?.length" class="crm-sec">
      <h3>{{ t('crm.writesFrom') }}</h3>
      <button v-for="c in p.companies" :key="c.id" type="button" class="crm-contact" @click="openParty(c.id)">
        <span class="crm-ico company"><AppIcon name="home" /></span>
        <span class="crm-txt"><b>{{ c.name }}<span v-if="c.own" class="crm-own">{{ t('crm.ownTag') }}</span></b><span>{{ relMeta(c) }}</span></span>
      </button>
    </section>

    <section v-for="g in linkGroups" :key="g.role" class="crm-sec">
      <h3>{{ g.title }}</h3>
      <button v-for="c in g.items" :key="c.id" type="button" class="crm-contact" @click="openParty(c.id)">
        <span class="crm-ico company"><AppIcon name="home" /></span>
        <span class="crm-txt"><b>{{ c.name }}<span v-if="c.own" class="crm-own">{{ t('crm.ownTag') }}</span></b><span>{{ relMeta(c) }}</span></span>
      </button>
    </section>

    <section v-if="req.length" class="crm-sec">
      <h3>{{ t('crm.requisites') }} <button class="link" @click="copy(reqText)">{{ t('crm.copyLower') }}</button></h3>
      <dl class="crm-kv"><template v-for="[k, v, c] in req" :key="k"><dt>{{ k }}</dt><dd>{{ v }}<button v-if="c" type="button" class="copy-mail" :title="t('reader.copyValue', { v })" :aria-label="t('reader.copyValue', { v })" @click="copy(v)"><AppIcon name="copy" /></button></dd></template></dl>
    </section>

    <section class="crm-sec">
      <h3>{{ t('crm.contacts') }}</h3>
      <dl class="crm-kv">
        <template v-if="p.phones.length"><dt>{{ t('common.phone') }}</dt><dd><a v-for="ph in p.phones" :key="ph" :href="'tel:' + ph.replace(/[^+\d]/g, '')" class="crm-pill">{{ ph }}</a></dd></template>
        <template v-if="p.emails.length"><dt>{{ t('crm.email') }}</dt><dd><span v-for="m in p.emails" :key="m" class="crm-pill">{{ m }}</span></dd></template>
        <template v-if="p.domains.length"><dt>{{ t('crm.domains') }}</dt><dd>{{ p.domains.join(', ') }}</dd></template>
        <template v-if="p.website"><dt>{{ t('crm.website') }}</dt><dd>{{ p.website }}</dd></template>
      </dl>
      <p v-if="!p.phones.length && !p.emails.length && !p.website" class="hint">{{ t('crm.noneYet') }}</p>
    </section>

    <section class="crm-sec">
      <h3>{{ t('crm.deliveryAddresses') }}</h3>
      <div v-for="a in p.addresses || []" :key="a.id" class="crm-addr">
        <span>{{ a.address }}</span>
        <button class="icon-btn" :title="t('crm.copy')" :aria-label="t('crm.copy')" @click="copy(a.address)"><AppIcon name="list" /></button>
        <button class="icon-btn" :title="t('common.delete')" :aria-label="t('common.delete')" @click="removeAddress(p.id, a.id)"><AppIcon name="close" /></button>
      </div>
      <form class="crm-addr-add" @submit.prevent="addAddr"><input v-model="newAddr" type="text" :placeholder="t('crm.addDeliveryAddress')" /><button class="btn sm" :disabled="newAddr.trim().length < 5">{{ t('common.add') }}</button></form>
    </section>

    <section v-if="p.contacts?.length" class="crm-sec">
      <h3>{{ p.own ? t('crm.employees') : t('crm.contactPersons') }}</h3>
      <button v-for="c in p.contacts" :key="c.id" type="button" class="crm-contact" @click="openParty(c.id)">
        <span class="crm-ico person"><AppIcon name="user" /></span>
        <span class="crm-txt"><b>{{ c.name }}</b><span>{{ [c.position, c.phones[0], c.emails[0], c.letters ? t('crm.letters', { n: c.letters }) : ''].filter(Boolean).join(' · ') }}</span></span>
      </button>
    </section>

    <section v-if="p.files?.length" class="crm-sec">
      <h3>{{ t('crm.filesFromLetters', { n: p.files.length }) }}</h3>
      <button v-for="f in p.files" :key="f.account + f.uid + f.part" type="button" class="crm-file" @click="openFile(f)">
        <span class="fp-ext" :style="{ background: extColor({ name: f.name, mime: f.mime } as any) }">{{ ext({ name: f.name, mime: f.mime } as any) || 'FILE' }}</span>
        <span class="crm-txt"><b>{{ f.name }}</b><span>{{ longDate(f.sentAt) }} · {{ fileSize(f.size) }}<template v-if="kindFile(f.kind)"> · {{ kindFile(f.kind) }}</template></span></span>
      </button>
    </section>

    <section v-if="p.history?.length" class="crm-sec">
      <h3>{{ t('crm.correspondence', { n: p.history.length }) }}</h3>
      <button v-for="l in p.history" :key="l.account + l.messageId" type="button" class="crm-letter" @click="openLetter(l)">
        <span class="crm-letter-date">{{ longDate(l.sentAt) }}</span><span>{{ l.subject || t('common.noSubject') }}</span>
        <span v-if="(mail.me?.accounts.length || 0) > 1" class="hint">{{ l.account }}</span>
      </button>
    </section>

    <section v-if="p.notes" class="crm-sec"><h3>{{ t('crm.notes') }}</h3><p class="crm-notes">{{ p.notes }}</p></section>
  </div>
  <div v-else-if="crm.currentLoading" class="crm-body" style="min-height:320px"></div>
</template>
