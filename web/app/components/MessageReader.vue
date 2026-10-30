<script setup lang="ts">
import { computed, watch, onMounted, onBeforeUnmount, nextTick, ref } from 'vue'
import { toast } from '~/composables/toast'
import { copyText } from '~/composables/dialogs'
import { formatPhone } from '~/composables/phone'
import { api } from '~/composables/api'
import { closePreview, setAV, RISK_TEXT } from '~/composables/attachments'
import { newEvent } from '~/composables/calendar'
import { ai, hasAI, aiReplyInto, aiSummary, replyPanel, openReplyPanel, aiName } from '~/composables/ai'
import { newFolder, addSenderToFolder, PUBLIC_DOMAINS } from '~/composables/folders'
import { riskyLinks } from '~/composables/links'
import { labelById, LABEL_COLORS } from '~/composables/labels'
import { crm, lookupSender, openParty, extractFromLetter, taxIdLabel, cardCountry, showRussianFields } from '~/composables/crm'
import { mail, act, startCompose, imagesAllowed, setImagePrefs, closeMessage, accountColor, accountOf, identityOf, openMessage, keyOf, type Attachment, type Summary } from '~/composables/mail'
import { longDate, displayName } from '~/composables/format'
import { isDark } from '~/composables/theme'
import { t } from '~/composables/i18n'

const m = computed(() => mail.current)
const summary = computed<Summary | null>(() => m.value)
// Toolbar actions (archive, delete, spam, unread) apply to the whole conversation in this folder.
const targets = computed<Summary[]>(() => {
  if (!m.value) return []
  const k = keyOf(m.value)
  return mail.threadItems.some(x => keyOf(x) === k) ? mail.threadItems : [m.value]
})
// Earlier letters of the conversation. Long ones are folded like Gmail: first, "N more", last.
const curIdx = computed(() => {
  const cur = m.value
  if (!cur) return -1
  return mail.thread.findIndex(x => keyOf(x) === keyOf(cur) || (!!cur.messageId && x.messageId === cur.messageId))
})
const earlier = computed(() => curIdx.value < 0 ? [] : mail.thread.slice(0, curIdx.value))
const later = computed(() => curIdx.value < 0 ? [] : mail.thread.slice(curIdx.value + 1))
const folded = computed(() => !mail.threadAll && earlier.value.length > 3)
const shownEarlier = computed(() => folded.value ? [] : earlier.value)
const isSpam = computed(() => !!m.value && (m.value.folder === folderOf('junk') || m.value.spam))
// Antivirus: findings of the letter (X-Virus) and risky file types, most serious first.
watch(m, x => { if (x) setAV(x, x.av) }, { immediate: true })
const imagesOn = computed(() => !!m.value && (mail.showImages || (!isSpam.value && imagesAllowed(m.value))))
async function trustImages() {
  try { await setImagePrefs({ allow: m.value!.from.email }); toast(t('reader.imagesTrusted', { email: m.value!.from.email })) }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}
const avWarnings = computed(() => {
  const x = m.value
  if (!x) return []
  const kinds = new Set<string>(x.av || [])
  for (const a of x.attachments) if (a.risk) kinds.add(a.risk)
  return (['virus', 'exec', 'macro', 'encrypted', 'unchecked', 'html', 'spam', 'heuristic'] as const)
    .filter(k => kinds.has(k)).map(k => ({ kind: k, title: RISK_TEXT[k][0], text: RISK_TEXT[k][1] }))
})
const inTrash = computed(() => !!m.value && m.value.folder === folderOf('trash'))
const canReply = computed(() => !!mail.me?.accounts.some(a => a.canSend))
function folderOf(role: string) {
  return m.value ? mail.folders[m.value.account]?.find(f => f.role === role)?.name : undefined
}

// A letter that failed the sender checks and carries a download / misleading link: warned above it.
const riskyCount = computed(() => m.value?.html ? riskyLinks(m.value.html) : 0)
const authFailed = computed(() => { const a = m.value?.auth || {}; return ['spf', 'dkim', 'dmarc'].some(k => a[k] && !['pass', 'none', ''].includes(a[k]!)) })
// The row that was clicked, while its letter loads.
// (A letter opened from the address bar has no list row yet: then only the veil.)
const pendingRow = computed(() => {
  if (!mail.selected) return undefined
  const r = mail.messages.find(x => keyOf(x) === mail.selected) || mail.threadItems.find(x => keyOf(x) === mail.selected)
  return r?.from?.email ? r : undefined
})
// Another letter opened: the pane starts at its subject, not where the previous one was scrolled.
const bodyEl = ref<HTMLElement | null>(null)
watch(() => mail.selected, () => { if (bodyEl.value) bodyEl.value.scrollTop = 0 }, { flush: 'post' })
watch(() => mail.currentLoading, on => { if (!on && bodyEl.value) bodyEl.value.scrollTop = 0 }, { flush: 'post' })
const authBadges = computed(() => {
  const a = m.value?.auth || {}
  const out: { cls: string, text: string }[] = []
  const pass = ['spf', 'dkim', 'dmarc'].filter(k => a[k] === 'pass')
  const fail = ['spf', 'dkim', 'dmarc'].filter(k => a[k] && a[k] !== 'pass' && a[k] !== 'none')
  if (pass.length) out.push({ cls: 'ok', text: t('reader.authPassed', { checks: pass.map(s => s.toUpperCase()).join(' · ') }) })
  if (fail.length) out.push({ cls: 'bad', text: t('reader.authFailed', { checks: fail.map(s => s.toUpperCase()).join(' · ') }) })
  return out
})

function back() { mail.reading = false }
async function copyEmail(e: string) { toast(await copyText(e) ? t('reader.copiedAddress', { v: e }) : t('reader.copyAddressFailed'), { ms: 2500 }) }
function jumpToAtts() {
  const el = document.getElementById('main-atts')
  el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el?.classList.remove('flash'); void el?.offsetWidth; el?.classList.add('flash')
}
// Reply written by the AI assistant; `said` is what the user typed or dictated ("say we ship tomorrow").
async function aiReply(said = '') {
  replyPanel.open = false
  await startCompose('reply')
  await nextTick()
  if (mail.compose) await aiReplyInto(mail.compose, said)
}
// The panel belongs to one letter: switching letters closes it.
watch(() => m.value && keyOf(m.value), () => { replyPanel.open = false })
const moveTargets = computed(() => m.value ? (mail.folders[m.value.account] || []).filter(f => (!f.role || f.role === 'archive' || f.role === 'inbox') && f.name !== m.value!.folder) : [])
// Move only this conversation, or also every letter from its sender (address or whole domain):
// the sender joins the folder's rule, and the rest of their letters follow on the server.
const moveScope = ref<'one' | 'sender' | 'domain' | 'base'>('one')
const senderDomain = computed(() => m.value?.from.email.split('@')[1]?.toLowerCase() || '')
// news.ozon.ru → ozon.ru: the whole company with all its subdomains.
const baseDomain = computed(() => { const p = senderDomain.value.split('.'); return p.length > 2 && !/^(com|net|org|co|msk|spb)$/.test(p[p.length - 2]!) ? p.slice(-2).join('.') : '' })
const canDomain = computed(() => !!senderDomain.value && !PUBLIC_DOMAINS.has(senderDomain.value))
const scopedTargets = computed(() => moveScope.value === 'one' ? moveTargets.value : moveTargets.value.filter(f => !f.role))
watch(() => m.value && keyOf(m.value), () => { moveScope.value = 'one' })
async function moveTo(folder: string, role?: string) {
  const x = m.value
  if (!x) return
  const scope = moveScope.value
  await act('move', targets.value, role === 'archive' ? 'archive' : role === 'inbox' ? 'inbox' : 'folder:' + folder)
  if (scope === 'one' || role) return
  const sender = scope === 'base' ? '@' + baseDomain.value : scope === 'domain' ? '@' + senderDomain.value : x.from.email
  working.value = t('reader.movingFrom', { sender, folder })
  try {
    await addSenderToFolder(x.account, folder, sender)
    toast(t('reader.ruleAdded', { sender, folder }), { ms: 7000 })
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { working.value = '' }
}
// Counterparty of the letter, shown under the sender line: the sender, or — in a letter we sent —
// the first recipient outside our own mailboxes.
const ownAddr = (e: string) => !!accountOf(e.toLowerCase()) || !!identityOf(e)
const partyEmail = computed(() => {
  const x = m.value
  if (!x) return ''
  if (!ownAddr(x.from.email)) return x.from.email.toLowerCase()
  return ([...x.to, ...(x.cc || [])].find(r => r.email && !ownAddr(r.email))?.email || '').toLowerCase()
})
const senderCard = computed(() => partyEmail.value ? crm.lookups[partyEmail.value] : undefined)
watch(partyEmail, e => { if (e) lookupSender(e) }, { immediate: true })
// All phones: the person's who wrote first, then the company's.
const partyPerson = computed(() => senderCard.value && senderCard.value !== 'loading' ? senderCard.value.contacts?.[0] : undefined)
const partyPhones = computed(() => {
  const c = senderCard.value
  if (!c || c === 'loading') return []
  return [...new Set([...(partyPerson.value?.phones || []), ...c.phones].map(formatPhone).filter(Boolean))]
})
// okKey: the message shown once copied, with the value as {v} (and the name of an identifier as {label}).
async function copyVal(okKey: string, v: string, label = '') { toast(await copyText(v) ? t(okKey, { v, label }) : t('reader.copyFailed'), { ms: 2500 }) }
// The company's identifiers for the box: the main one named after its country, KPP / OGRN of a
// Russian company, then the others ({"VAT": "DE123…"}).
const partyIds = computed(() => {
  const c = senderCard.value
  if (!c || c === 'loading' || c.kind !== 'company') return []
  const out: { k: string, v: string }[] = []
  if (c.inn) out.push({ k: taxIdLabel(cardCountry(c)), v: c.inn })
  if (showRussianFields(c)) {
    if (c.kpp) out.push({ k: t('crm.kpp'), v: c.kpp })
    if (c.ogrn) out.push({ k: t('crm.ogrn'), v: c.ogrn })
  }
  for (const [k, v] of Object.entries(c.ids || {})) if (v) out.push({ k, v })
  return out
})
const extracting = ref(false)
// A longer action on the open letter: the reading pane is veiled with what is being done.
const working = ref('')
async function makeCard() {
  if (!m.value) return
  extracting.value = true
  working.value = t('reader.aiMakingCard', { ai: aiName() })
  try { const id = await extractFromLetter(m.value); delete crm.lookups[partyEmail.value]; await lookupSender(partyEmail.value); closeMessage(); openParty(id) }
  catch (e: any) { toast(e.message, { kind: 'err', ms: 8000 }) } finally { extracting.value = false; working.value = '' }
}
function showCard(id: number) { closeMessage(); openParty(id) }
// Company-wide black / white list, straight from a letter.
async function senderRule(kind: 'block' | 'allow', domain = false) {
  const x = m.value
  if (!x) return
  const entry = domain ? '@' + x.from.email.split('@')[1] : (x.from.name ? `${x.from.name} <${x.from.email}>` : x.from.email)
  working.value = kind === 'block' ? t('reader.blocking') : t('reader.trusting')
  try {
    await api('/api/sender-rules', { method: 'POST', body: { kind, entries: [entry], note: t('reader.ruleNote', { subject: x.subject || t('reader.noSubjectPlain') }) } })
    if (kind === 'block') { await act('move', targets.value, 'junk'); toast(t('reader.blocked', { entry })) }
    else { if (isSpam.value) await act('move', targets.value, 'inbox'); toast(t('reader.trusted', { entry })) }
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { working.value = '' }
}
const aiSum = computed(() => m.value ? ai.summary[keyOf(m.value)] : undefined)
// A letter becomes a calendar entry: subject as the title, sender and first lines as notes.
function toCalendar() {
  const x = m.value
  if (!x) return
  const text = ((x as any).text || '').trim().slice(0, 600)
  newEvent(undefined, { title: x.subject || t('reader.letter'), mailRef: keyOf(x), notes: t('reader.calendarNotes', { from: x.from.name ? `${x.from.name} <${x.from.email}>` : x.from.email }) + (text ? '\n\n' + text : '') })
}
// Esc closes the letter when nothing else (preview, dialog, compose, menu) is using it.
function onEsc(e: KeyboardEvent) {
  if (e.key !== 'Escape' || e.defaultPrevented || !mail.current) return
  const el = e.target as HTMLElement
  if (el.closest('input,textarea,[contenteditable="true"],.modal,.scrim,.cw,.tp-pop,.sel-list') || document.querySelector('.scrim,.ac-list,.sel-list,.tp-pop')) return
  closeMessage()
}
onMounted(() => window.addEventListener('keydown', onEsc))
onBeforeUnmount(() => window.removeEventListener('keydown', onEsc))
watch(() => m.value?.uid, closePreview)
// "New message in this conversation from <b>name</b>": the sentence around the bold name.
const threadNewText = computed(() => t('reader.threadNew', { name: '\u0000' }).split('\u0000'))
</script>

<template>
  <section class="reader" :aria-label="t('reader.letter')">
    <CrmView v-if="!m && !mail.currentLoading && !mail.currentError && crm.open" />
    <CalendarView v-else-if="!m && !mail.currentLoading && !mail.currentError" />

    <template v-else>
      <div class="reader-bar">
        <button class="icon-btn back-btn" :aria-label="t('reader.backToList')" @click="back"><AppIcon name="back" /></button>
        <template v-if="m && summary">
          <button class="icon-btn" :title="t('list.archive')" :aria-label="t('list.archive')" @click="act('move', targets, 'archive')"><AppIcon name="archive" /></button>
          <button class="icon-btn" :title="inTrash ? t('list.deleteForever') : t('common.delete')" :aria-label="t('common.delete')" @click="act('delete', targets)"><AppIcon name="trash" /></button>
          <button v-if="!isSpam" class="icon-btn" :title="t('list.isSpam')" :aria-label="t('list.isSpam')" @click="act('move', targets, 'junk')"><AppIcon name="junk" /></button>
          <button v-else class="icon-btn" :title="t('list.notSpam')" :aria-label="t('list.notSpam')" @click="act('move', targets, 'inbox')"><AppIcon name="check" /></button>
          <button class="icon-btn" :title="t('list.markUnread')" :aria-label="t('list.markUnread')" @click="act('unseen', targets); back()"><AppIcon name="mail" /></button>
          <button class="icon-btn" :title="m.flagged ? t('list.unflag') : t('list.flag')" :aria-label="m.flagged ? t('list.unflag') : t('list.flag')" @click="act(m.flagged ? 'unflag' : 'flag', [summary])"><AppIcon name="star" :fill="m.flagged" /></button>
          <button v-if="hasAI('summary')" class="icon-btn" :title="t('reader.summaryTitle', { ai: aiName() })" :aria-label="t('reader.summary')" :disabled="aiSum === 'loading'" @click="aiSummary"><AppIcon name="sparkle" /></button>
          <LabelPicker :items="targets" />
          <ToolPop v-slot="{ close }" :title="t('list.moveToFolder')" icon="folder" :down="true" align="right">
            <div class="move-list">
              <div class="move-scope" role="radiogroup" :aria-label="t('reader.moveWhat')">
                <label><input v-model="moveScope" type="radio" value="one" /> {{ t('reader.scopeOne') }}</label>
                <label :title="t('reader.scopeSenderHint', { email: m!.from.email })"><input v-model="moveScope" type="radio" value="sender" /> {{ t('reader.scopeSender', { email: m!.from.email }) }}</label>
                <label v-if="canDomain" :title="t('reader.scopeDomainHint', { domain: senderDomain })"><input v-model="moveScope" type="radio" value="domain" /> {{ t('reader.scopeDomain', { domain: senderDomain }) }}</label>
                <label v-if="canDomain && baseDomain" :title="t('reader.scopeBaseHint', { domain: baseDomain })"><input v-model="moveScope" type="radio" value="base" /> {{ t('reader.scopeBase', { domain: baseDomain }) }}</label>
              </div>
              <button v-for="f in scopedTargets" :key="f.name" type="button" class="tp-item" @click="close(); moveTo(f.name, f.role)">
                <AppIcon :name="f.role === 'archive' ? 'archive' : f.role === 'inbox' ? 'inbox' : 'folder'" /> {{ f.role === 'archive' ? t('list.folderArchive') : f.role === 'inbox' ? t('list.folderInbox') : f.name }}</button>
              <p v-if="moveScope !== 'one' && !scopedTargets.length" class="hint" style="padding:4px 10px">{{ t('reader.createFolderHint') }}</p>
              <button type="button" class="tp-item" style="color:var(--accent)" @click="close(); newFolder(m!.account)"><AppIcon name="plus" /> {{ t('reader.newFolder') }}</button>
            </div>
          </ToolPop>
          <ToolPop v-slot="{ close }" :title="t('reader.senderMenu')" icon="shield" :down="true">
            <div class="move-list">
              <button type="button" class="tp-item" @click="close(); senderRule('block')"><AppIcon name="shieldX" /> {{ t('reader.block', { email: m!.from.email }) }}</button>
              <button v-if="mail.me?.isAdmin" type="button" class="tp-item" @click="close(); senderRule('block', true)"><AppIcon name="shieldX" /> {{ t('reader.blockDomain', { domain: m!.from.email.split('@')[1] }) }}</button>
              <button type="button" class="tp-item" @click="close(); senderRule('allow')"><AppIcon name="shield" /> {{ t('reader.trust') }}</button>
            </div>
          </ToolPop>
          <button class="icon-btn" :title="t('reader.addToCalendar')" :aria-label="t('reader.addToCalendar')" @click="toCalendar"><AppIcon name="cal" /></button>
          <span class="grow"></span>
          <button v-if="canReply" class="icon-btn" :title="t('reader.reply')" :aria-label="t('reader.reply')" @click="startCompose('reply')"><AppIcon name="reply" /></button>
        </template>
        <span v-if="!(m && summary)" class="grow"></span>
        <button class="icon-btn close-btn" :title="t('reader.closeLetterEsc')" :aria-label="t('reader.closeLetter')" @click="closeMessage"><AppIcon name="close" /></button>
      </div>

      <div class="reader-main veil-host">
      <LoadingVeil :show="mail.currentLoading || !!working" :text="working || t('reader.loading')" :slow-text="working ? undefined : t('reader.loadingSlow')" />
      <div ref="bodyEl" class="reader-body">
        <div v-if="mail.threadNew" class="thread-new" role="status">
          <AppIcon name="mail" />
          <span>{{ threadNewText[0] }}<b>{{ mail.threadNew.from.name || mail.threadNew.from.email }}</b>{{ threadNewText[1] }}</span>
          <button class="btn sm primary" @click="openMessage(mail.threadNew!)">{{ t('reader.show') }}</button>
          <button class="icon-btn" :aria-label="t('reader.hide')" :title="t('reader.hide')" @click="mail.threadNew = null"><AppIcon name="close" /></button>
        </div>
        <div v-if="mail.currentLoading" class="reader-inner" aria-busy="true">
          <!-- What the list already knows is shown at once; the text and files come under it. -->
          <template v-if="pendingRow">
            <h1 class="subject">{{ pendingRow.subject || t('common.noSubject') }}</h1>
            <div class="pending-from">
              <AppAvatar :email="pendingRow.from.email" :name="displayName(pendingRow.from)" />
              <span><b>{{ displayName(pendingRow.from) }}</b> <span class="hint">&lt;{{ pendingRow.from.email }}&gt;</span></span>
              <span class="grow"></span><span class="hint">{{ longDate(pendingRow.date) }}</span>
            </div>
            <p v-if="pendingRow.preview" class="pending-preview">{{ pendingRow.preview }}</p>
          </template>
          <div style="min-height:240px"></div>
        </div>
        <div v-else-if="mail.currentError" class="empty">
          <p>{{ mail.currentError }}</p>
        </div>
        <div v-else-if="m" class="reader-inner">
          <h2 class="subject">{{ m.subject || t('common.noSubject') }}<span v-for="id in (m.labels || []).filter(i => labelById(i))" :key="id" class="lbl big" :style="{ background: LABEL_COLORS[labelById(id)!.color]?.bg, color: LABEL_COLORS[labelById(id)!.color]?.fg }">{{ labelById(id)!.name }}<button type="button" :aria-label="t('reader.removeLabel', { name: labelById(id)!.name })" @click="act('unlabel', targets, String(id))"><AppIcon name="close" /></button></span><span v-if="mail.thread.length > 1" class="thread-count" :title="t('list.inThread', { n: mail.thread.length })">{{ mail.thread.length }}</span></h2>
          <div v-if="earlier.length" class="thread">
            <template v-if="folded">
              <ThreadItem :m="earlier[0]" />
              <button type="button" class="thread-more" :title="t('reader.showMore', { n: earlier.length - 2 })" @click="mail.threadAll = true">
                <span>{{ earlier.length - 2 }}</span></button>
              <ThreadItem :m="earlier[earlier.length - 1]" />
            </template>
            <ThreadItem v-for="x in shownEarlier" :key="keyOf(x)" :m="x" />
          </div>
          <div class="sender">
            <AppAvatar :email="m.from.email" :name="displayName(m.from)" :bimi="m.auth?.dmarc === 'pass' || m.dmarc === 'pass'" />
            <div class="sender-info">
              <div><span class="sender-name">{{ displayName(m.from) }}</span><span v-if="m.from.name" class="sender-addr">&lt;{{ m.from.email }}&gt;</span>
                <button v-if="m.from.email" type="button" class="copy-mail" :title="t('reader.copyValue', { v: m.from.email })" :aria-label="t('reader.copyValue', { v: m.from.email })" @click="copyEmail(m.from.email)"><AppIcon name="copy" /></button></div>
              <div class="to-line">
                <span>{{ t('reader.to') }}</span>
                <span v-for="r in m.to.slice(0, 4)" :key="r.email" class="acct-chip addr-chip">
                  <span v-if="accountOf(r.email)" class="dot" :style="{ background: accountColor(r.email) }"></span>
                  <template v-if="r.name && r.name !== r.email">{{ r.name }} <span class="addr-mail">{{ r.email }}</span></template><template v-else>{{ r.email }}</template>
                  <button type="button" class="copy-mail" :title="t('reader.copyValue', { v: r.email })" :aria-label="t('reader.copyValue', { v: r.email })" @click="copyEmail(r.email)"><AppIcon name="copy" /></button>
                </span>
                <span v-if="m.to.length > 4">{{ t('reader.andMore', { n: m.to.length - 4 }) }}</span>
              </div>
              <div v-if="m.cc?.length" class="to-line">
                <span>{{ t('reader.cc') }}</span>
                <span v-for="r in m.cc" :key="r.email" class="acct-chip addr-chip">
                  <template v-if="r.name && r.name !== r.email">{{ r.name }} <span class="addr-mail">{{ r.email }}</span></template><template v-else>{{ r.email }}</template>
                  <button type="button" class="copy-mail" :title="t('reader.copyValue', { v: r.email })" :aria-label="t('reader.copyValue', { v: r.email })" @click="copyEmail(r.email)"><AppIcon name="copy" /></button>
                </span>
              </div>
              <div v-if="!accountOf(m.to[0]?.email || '') && (mail.me?.accounts.length || 0) > 1" class="to-line">
                <span>{{ t('reader.inMailbox') }}</span><span class="acct-chip"><span class="dot" :style="{ background: accountColor(m.account) }"></span>{{ m.account }}</span>
              </div>
            </div>
            <span class="sender-date">{{ longDate(m.date) }}</span>
          </div>
          <div v-if="senderCard && senderCard !== 'loading' && !senderCard.own" class="party-box">
            <button type="button" class="pb-name" :title="t('reader.openCard', { name: senderCard.name })" @click="showCard(senderCard.id)">
              <AppIcon :name="senderCard.kind === 'company' ? 'home' : 'user'" /><b>{{ senderCard.name }}</b>
              <span v-if="partyPerson" class="pb-person">{{ partyPerson.name }}{{ partyPerson.position ? ', ' + partyPerson.position : '' }}</span>
              <span class="pb-count">{{ t('reader.lettersCount', { n: senderCard.letters }) }}</span>
              <i v-if="senderCard.pending" class="crm-dot" :title="t('reader.pendingData')"></i>
            </button>
            <div class="pb-fields">
              <span v-for="x in partyIds" :key="x.k" class="pb-f"><span class="pb-k">{{ x.k }}</span><span class="tnum">{{ x.v }}</span>
                <button type="button" class="copy-mail" :title="t('reader.copyId', { label: x.k })" :aria-label="t('reader.copyId', { label: x.k })" @click="copyVal('reader.copiedId', x.v, x.k)"><AppIcon name="copy" /></button></span>
              <span v-for="ph in partyPhones" :key="ph" class="pb-f"><span class="pb-k">{{ t('reader.phoneShort') }}</span><a class="tnum" :href="'tel:' + ph.replace(/[^+\d]/g, '')">{{ ph }}</a>
                <button type="button" class="copy-mail" :title="t('reader.copyValue', { v: ph })" :aria-label="t('reader.copyPhone')" @click="copyVal('reader.copiedPhone', ph)"><AppIcon name="copy" /></button></span>
              <span class="pb-f"><span class="pb-k">{{ t('reader.emailShort') }}</span><span>{{ partyEmail }}</span>
                <button type="button" class="copy-mail" :title="t('reader.copyValue', { v: partyEmail })" :aria-label="t('reader.copyEmail')" @click="copyVal('reader.copiedAddress', partyEmail)"><AppIcon name="copy" /></button></span>
            </div>
          </div>
          <div v-else-if="senderCard === null && hasAI('reply')" class="party-chip-row">
            <button type="button" class="party-chip add" :disabled="extracting" @click="makeCard">
              <AppIcon name="plus" />{{ extracting ? t('reader.aiReadingAtts', { ai: aiName() }) : t('reader.makeCard') }}</button>
          </div>

          <button v-if="m.attachments.length" type="button" class="att-jump" @click="jumpToAtts">
            <AppIcon name="clip" /><b>{{ m.attachments.length }}</b>
            <span>{{ m.attachments.length > 3 ? t('reader.filesAndMore', { names: m.attachments.slice(0, 3).map(a => a.name).join(', '), n: m.attachments.length - 3 }) : m.attachments.map(a => a.name).join(', ') }}</span>
            <AppIcon name="chev" />
          </button>
          <div v-if="riskyCount && authFailed" class="risk-note hard" role="alert">
            <AppIcon name="shieldX" />
            <span>{{ t('reader.riskyLinks', { n: riskyCount }) }}</span>
          </div>
          <div v-if="authBadges.length || m.sentBy" class="auth">
            <span v-for="b in authBadges" :key="b.text" class="badge" :class="b.cls"><AppIcon :name="b.cls === 'ok' ? 'shield' : 'shieldX'" />{{ b.text }}</span>
            <span v-if="m.sentBy" class="sent-by"><AppIcon name="user" />{{ m.sentBy.name ? t('reader.sentByNamed', { login: m.sentBy.login, name: m.sentBy.name }) : t('reader.sentBy', { login: m.sentBy.login }) }}</span>
          </div>
          <div v-for="w in avWarnings" :key="w.kind" class="notice av-warn" :class="w.kind === 'virus' || w.kind === 'exec' || w.kind === 'encrypted' || w.kind === 'unchecked' ? 'danger' : 'warn'">
            <AppIcon :name="w.kind === 'virus' ? 'shieldX' : 'alert'" /><span><b>{{ w.title }}.</b> {{ w.text }}</span>
          </div>
          <div v-if="isSpam" class="notice danger"><AppIcon name="junk" />{{ t('reader.spamNotice') }}</div>
          <div v-if="m.remoteImages && !imagesOn" class="notice">
            <AppIcon name="image" />
            <span>{{ t('reader.imagesHidden') }}</span>
            <button class="link" @click="mail.showImages = true">{{ t('reader.showImages') }}</button>
            <button v-if="!isSpam && m.from.email" class="link" @click="trustImages">{{ t('reader.alwaysShowFrom', { email: m.from.email }) }}</button>
          </div>

          <div v-if="isDark" class="frame-tools">
            <button class="link" @click="mail.originalColors = !mail.originalColors">
              {{ mail.originalColors ? t('reader.darkColors') : t('reader.originalColors') }}</button>
          </div>
          <div v-if="aiSum" class="ai-box veil-host" :class="{ 't-loading': aiSum === 'loading' }" aria-live="polite"><b><AppIcon name="sparkle" />{{ t('reader.summaryHead', { ai: aiName() }) }}</b><LoadingVeil v-if="aiSum === 'loading'" class="compact" show :text="t('reader.aiReading', { ai: aiName() })" /><template v-else>{{ aiSum }}</template></div>
          <MessageFrame :html="m.html" :allow-remote="imagesOn" :original="mail.originalColors" />

          <AttachmentCards v-if="m.attachments.length" id="main-atts" :msg="m" :atts="m.attachments" />

          <div v-if="later.length" class="thread later-items">
            <ThreadItem v-for="x in later" :key="keyOf(x)" :m="x" />
          </div>

          <ComposeForm v-if="mail.compose && !mail.composeModal" />
          <div v-else-if="canReply" class="reply-bar">
            <button class="btn" @click="startCompose('reply')"><AppIcon name="reply" /><span>{{ t('reader.reply') }}</span></button>
            <button class="btn" :title="t('reader.replyAll')" @click="startCompose('replyall')"><AppIcon name="replyAll" /><span class="rb-t">{{ t('reader.replyAll') }}</span></button>
            <button class="btn" :title="t('reader.forward')" @click="startCompose('fwd')"><AppIcon name="fwd" /><span class="rb-t">{{ t('reader.forward') }}</span></button>
            <template v-if="hasAI('reply')">
              <button class="btn ai" :disabled="!!ai.busy" :title="t('reader.aiReplyHint', { ai: aiName() })" @click="hasAI('reply_questions') ? openReplyPanel() : aiReply()"><AppIcon name="sparkle" /><span class="rb-t">{{ t('reader.aiReply') }}</span></button>
              <VoiceButton id="reader" :label="t('reader.voiceReply')" @text="aiReply" />
            </template>
          </div>
          <AiReplyPanel v-if="replyPanel.open && replyPanel.key === keyOf(m)" @write="aiReply" @close="replyPanel.open = false" />
        </div>
      </div>
      </div>
      <!-- The inline answer's Send, attach and formatting: always at the bottom, outside the scroll. -->
      <div id="reader-dock" class="reader-dock composer"></div>
    </template>

  </section>
</template>
