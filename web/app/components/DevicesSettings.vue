<script setup lang="ts">
// Phones and mail programs: a separate app password for each device (revocable here), a guided
// setup per device kind and the profile that sets up an iPhone or a Mac in one tap
// (mail of the chosen mailboxes, calendars and counterparties).
import { ref, computed, onMounted } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { confirmDialog, copyText } from '~/composables/dialogs'
import { deviceProfile, downloadBlob, MAIL_HOST } from '~/composables/mailclient'
import { t, fmtDate } from '~/composables/i18n'

type AppPass = { id: number, name: string, mailboxes: string[], created: string, lastUsed: string | null, lastIp: string }
type Kind = 'apple' | 'mac' | 'android' | 'outlook' | 'thunderbird' | 'other'

const items = ref<AppPass[]>([])
const loading = ref(true)
const busy = ref(false)
const login = ref(mail.me?.login || '')
const davURL = computed(() => `https://${MAIL_HOST}/dav/`)

async function load() {
  loading.value = true
  try { const r = await api<{ items: AppPass[], login: string }>('/api/app-passwords'); items.value = r.items; login.value = r.login }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { loading.value = false }
}
onMounted(load)

const KINDS = computed<{ id: Kind, name: string, icon: string, def: string }[]>(() => [
  { id: 'apple', name: t('devices.kindApple'), icon: 'phone', def: 'iPhone' },
  { id: 'android', name: t('devices.kindAndroid'), icon: 'phone', def: 'Android' },
  { id: 'mac', name: t('devices.kindMac'), icon: 'laptop', def: 'Mac' },
  { id: 'outlook', name: 'Outlook', icon: 'laptop', def: 'Outlook' },
  { id: 'thunderbird', name: 'Thunderbird', icon: 'laptop', def: 'Thunderbird' },
  { id: 'other', name: t('devices.kindOther'), icon: 'settings', def: '' },
])
const step = ref<'list' | 'kind' | 'form' | 'done'>('list')
const kind = ref<Kind>('apple')
const form = ref({ name: '', all: true, boxes: [] as string[] })
const created = ref<{ password: string, item: AppPass } | null>(null)
const boxes = computed(() => mail.me?.accounts || [])
const chosenBoxes = computed(() => form.value.all ? boxes.value : boxes.value.filter(a => form.value.boxes.includes(a.address)))

function start() { step.value = 'kind'; created.value = null }
function pickKind(k: Kind) {
  kind.value = k
  form.value = { name: KINDS.value.find(x => x.id === k)?.def || '', all: true, boxes: boxes.value.map(a => a.address) }
  step.value = 'form'
}
async function create() {
  if (!form.value.name.trim()) { toast(t('devices.nameRequired'), { kind: 'err' }); return }
  if (!chosenBoxes.value.length) { toast(t('devices.boxesRequired'), { kind: 'err' }); return }
  busy.value = true
  try {
    created.value = await api('/api/app-passwords', { method: 'POST', body: { name: form.value.name.trim(), mailboxes: form.value.all ? [] : form.value.boxes } })
    step.value = 'done'
    load()
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
async function revoke(p: AppPass) {
  if (!await confirmDialog(t('devices.revokeConfirm', { name: p.name }), t('devices.revokeText'), { ok: t('devices.revoke'), danger: true })) return
  try { await api(`/api/app-passwords/${p.id}`, { method: 'DELETE' }); toast(t('devices.revoked', { name: p.name })); load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function copy(v: string) { if (await copyText(v)) toast(t('common.copied')) }

function profileBlob() {
  return deviceProfile({ title: t('devices.profileTitle', { name: form.value.name }), login: login.value, password: created.value!.password,
    boxes: chosenBoxes.value.map(a => ({ address: a.address, name: a.name })), dav: true })
}
function downloadProfile() { downloadBlob(profileBlob(), `${MAIL_HOST}-${form.value.name.replace(/[^\p{L}\d-]+/gu, '_')}.mobileconfig`) }
// The phone opens the profile from the letter in its Mail app.
const mailing = ref(false)
async function mailProfile() {
  const box = boxes.value.find(a => a.canSend && chosenBoxes.value.some(c => c.address === a.address)) || boxes.value.find(a => a.canSend)
  if (!box) return
  mailing.value = true
  try {
    const fd = new FormData()
    fd.append('from', box.address)
    fd.append('to', box.address)
    fd.append('subject', t('devices.profileMailSubject', { name: form.value.name }))
    fd.append('html', `<p>${t('devices.profileMailText')}</p>`)
    fd.append('text', t('devices.profileMailText'))
    fd.append('files', new File([profileBlob()], `${form.value.name.replace(/[^\p{L}\d-]+/gu, '_') || 'TMAIL'}.mobileconfig`, { type: 'application/x-apple-aspen-config' }))
    await api('/api/send', { method: 'POST', body: fd })
    toast(t('devices.profileMailed', { address: box.address }))
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { mailing.value = false }
}
const fmt = (d: string | null) => d ? fmtDate(d, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : t('devices.never')
const boxNames = (p: AppPass) => p.mailboxes.length ? p.mailboxes.join(', ') : t('devices.allBoxes')
// The settings typed by hand in a program that does not set itself up.
const manual = computed(() => [
  { k: t('devices.server'), v: MAIL_HOST },
  { k: 'IMAP', v: `${MAIL_HOST} · 993 · SSL/TLS` },
  { k: 'SMTP', v: `${MAIL_HOST} · 465 · SSL/TLS` },
  { k: t('devices.davUrl'), v: davURL.value },
  { k: t('devices.davLogin'), v: login.value },
])
</script>

<template>
  <div class="dev veil-host">
    <LoadingVeil :show="loading || busy" :text="busy ? t('common.saving') : t('devices.loading')" />

    <template v-if="step === 'list'">
      <div class="dev-intro">
        <div>
          <b>{{ t('devices.title') }}</b>
          <p class="hint">{{ t('devices.intro') }}</p>
        </div>
        <button class="btn primary" @click="start"><AppIcon name="plus" />{{ t('devices.connect') }}</button>
      </div>
      <div v-if="items.length" class="dev-list">
        <div v-for="p in items" :key="p.id" class="dev-item">
          <span class="dev-ico"><AppIcon :name="/mac|outlook|thunder|windows|pc|ноут|комп/i.test(p.name) ? 'laptop' : 'phone'" /></span>
          <div class="dev-main">
            <b>{{ p.name }}</b>
            <span class="hint">{{ boxNames(p) }}</span>
            <span class="hint">{{ t('devices.createdAt', { date: fmt(p.created) }) }} · {{ p.lastUsed ? t('devices.lastUsed', { date: fmt(p.lastUsed), ip: p.lastIp }) : t('devices.notUsedYet') }}</span>
          </div>
          <button class="btn sm danger-text" @click="revoke(p)"><AppIcon name="trash" />{{ t('devices.revoke') }}</button>
        </div>
      </div>
      <p v-else-if="!loading" class="dev-empty hint">{{ t('devices.empty') }}</p>
      <details class="dev-manual">
        <summary>{{ t('devices.manualTitle') }}</summary>
        <p class="hint">{{ t('settings.clientsHint') }}</p>
        <MailClientCard v-for="a in mail.me?.accounts || []" :key="a.address" :address="a.address" :name="a.name" />
      </details>
    </template>

    <template v-else-if="step === 'kind'">
      <div class="dev-step-head"><button class="icon-btn" :aria-label="t('common.back')" @click="step = 'list'"><AppIcon name="back" /></button><b>{{ t('devices.whichDevice') }}</b></div>
      <div class="dev-kinds">
        <button v-for="k in KINDS" :key="k.id" type="button" class="dev-kind" @click="pickKind(k.id)"><AppIcon :name="k.icon" /><span>{{ k.name }}</span></button>
      </div>
    </template>

    <template v-else-if="step === 'form'">
      <div class="dev-step-head"><button class="icon-btn" :aria-label="t('common.back')" @click="step = 'kind'"><AppIcon name="back" /></button><b>{{ KINDS.find(k => k.id === kind)?.name }}</b></div>
      <form class="form dev-form" @submit.prevent="create">
        <label class="f">{{ t('devices.deviceName') }}<input v-model="form.name" type="text" maxlength="80" :placeholder="t('devices.deviceNamePlaceholder')" /></label>
        <div class="f">
          <span>{{ t('devices.mailboxes') }}</span>
          <label class="check"><input v-model="form.all" type="checkbox" />{{ t('devices.allBoxes') }}</label>
          <div v-if="!form.all" class="dev-boxes">
            <label v-for="a in boxes" :key="a.address" class="check"><input v-model="form.boxes" type="checkbox" :value="a.address" />{{ a.address }}</label>
          </div>
        </div>
        <p class="hint">{{ kind === 'apple' || kind === 'mac' ? t('devices.formHintApple') : kind === 'outlook' ? t('devices.formHintOutlook') : t('devices.formHint') }}</p>
        <div class="dev-foot"><button class="btn primary" :disabled="busy"><AppIcon name="key" />{{ t('devices.createPassword') }}</button></div>
      </form>
    </template>

    <template v-else-if="step === 'done' && created">
      <div class="dev-step-head"><AppIcon name="check" class="ok-ico" /><b>{{ t('devices.ready', { name: created.item.name }) }}</b></div>
      <div class="dev-pass">
        <span class="hint">{{ t('devices.passwordLabel') }}</span>
        <code>{{ created.password }}</code>
        <button class="btn sm" @click="copy(created.password)"><AppIcon name="copy" />{{ t('common.copy') }}</button>
      </div>
      <p class="mcc-warn">{{ t('devices.passwordOnce') }}</p>

      <div v-if="kind === 'apple' || kind === 'mac'" class="dev-how">
        <div class="dev-actions">
          <button class="btn primary" @click="downloadProfile"><AppIcon name="download" />{{ t('devices.downloadProfile') }}</button>
          <button class="btn" :disabled="mailing" @click="mailProfile"><AppIcon name="mail" />{{ mailing ? t('compose.sending') : t('devices.mailProfile') }}</button>
        </div>
        <ol>
          <li>{{ kind === 'apple' ? t('devices.appleStep1') : t('devices.macStep1') }}</li>
          <li>{{ kind === 'apple' ? t('devices.appleStep2') : t('devices.macStep2') }}</li>
          <li>{{ t('devices.appleStep3') }}</li>
        </ol>
      </div>
      <div v-else-if="kind === 'android'" class="dev-how">
        <ol>
          <li>{{ t('devices.androidMail', { host: MAIL_HOST }) }}</li>
          <li>{{ t('devices.androidDav') }}</li>
        </ol>
      </div>
      <div v-else-if="kind === 'outlook'" class="dev-how">
        <ol>
          <li>{{ t('devices.outlookMail') }}</li>
          <li>{{ t('devices.outlookCalendar') }}</li>
        </ol>
      </div>
      <div v-else-if="kind === 'thunderbird'" class="dev-how">
        <ol>
          <li>{{ t('devices.tbMail') }}</li>
          <li>{{ t('devices.tbDav') }}</li>
        </ol>
      </div>
      <dl class="dev-manual-rows">
        <template v-for="r in manual" :key="r.k">
          <dt>{{ r.k }}</dt><dd><span>{{ r.v }}</span><button type="button" class="icon-btn" :aria-label="t('mailclient.copyField', { label: r.k })" @click="copy(r.v)"><AppIcon name="copy" /></button></dd>
        </template>
        <dt>{{ t('devices.mailLogin') }}</dt><dd><span>{{ chosenBoxes.map(a => a.address).join(', ') }}</span></dd>
      </dl>
      <div class="dev-foot"><button class="btn" @click="step = 'list'">{{ t('common.done') }}</button></div>
    </template>
  </div>
</template>

<style scoped>
.dev{display:flex;flex-direction:column;gap:14px;min-height:260px}
.dev-intro{display:flex;align-items:flex-start;gap:16px;justify-content:space-between;flex-wrap:wrap}
.dev-intro p{margin:4px 0 0;max-width:640px}
.dev-list{display:flex;flex-direction:column;border:1px solid var(--line);border-radius:6px;overflow:hidden}
.dev-item{display:flex;align-items:center;gap:12px;padding:10px 14px;border-top:1px solid var(--line);background:var(--surface)}
.dev-item:first-child{border-top:0}
.dev-ico{width:36px;height:36px;border-radius:50%;background:var(--accent-soft);color:var(--accent);display:grid;place-items:center;flex:none}
.dev-ico svg{width:18px;height:18px}
.dev-main{display:flex;flex-direction:column;gap:1px;min-width:0;flex:1}
.dev-main .hint{font-size:12.5px;overflow:hidden;text-overflow:ellipsis}
.dev-empty{margin:0;padding:18px;border:1px dashed var(--line);border-radius:6px;text-align:center}
.dev-manual summary{cursor:pointer;color:var(--accent);font-size:13.5px}
.dev-manual{display:flex;flex-direction:column;gap:10px}
.dev-manual[open] > :not(summary){margin-top:10px}
.dev-step-head{display:flex;align-items:center;gap:8px;font-size:15px}
.dev-step-head .ok-ico{width:22px;height:22px;color:var(--ok)}
.dev-kinds{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:10px}
.dev-kind{display:flex;flex-direction:column;align-items:center;gap:8px;padding:18px 10px;border:1px solid var(--line);border-radius:6px;background:var(--surface);cursor:pointer;color:var(--ink);font-weight:500}
.dev-kind:hover{border-color:var(--accent);background:var(--accent-soft)}
.dev-kind svg{width:26px;height:26px;color:var(--accent)}
.dev-form{max-width:520px}
.dev-boxes{display:flex;flex-direction:column;gap:4px;padding:6px 0 0 4px}
.dev-foot{display:flex;justify-content:flex-end;gap:8px}
.dev-pass{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:12px 14px;border-radius:6px;background:var(--accent-soft)}
.dev-pass code{font-family:var(--f-mono);font-size:20px;letter-spacing:.06em;font-weight:600;user-select:all}
.mcc-warn{margin:0;font-size:13px;color:var(--warn)}
.dev-how{display:flex;flex-direction:column;gap:10px}
.dev-how ol{margin:0;padding-left:20px;display:flex;flex-direction:column;gap:6px;font-size:14px;color:var(--ink-2)}
.dev-actions{display:flex;gap:8px;flex-wrap:wrap}
.dev-manual-rows{display:grid;grid-template-columns:minmax(150px,32%) 1fr;gap:6px 14px;margin:0;font-size:14px;padding:12px 14px;border:1px solid var(--line);border-radius:6px}
.dev-manual-rows dt{color:var(--ink-3)}
.dev-manual-rows dd{margin:0;display:flex;align-items:center;gap:4px;font-weight:600;min-width:0;word-break:break-all}
.dev-manual-rows .icon-btn{width:24px;height:24px;flex:none}
.dev-manual-rows .icon-btn svg{width:14px;height:14px}
@media (max-width:760px){.dev-manual-rows{grid-template-columns:1fr}.dev-item{flex-wrap:wrap}}
</style>
