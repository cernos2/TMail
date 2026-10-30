<script setup lang="ts">
// Mail collector: external mailboxes whose mail is brought into ours (with folders and read marks).
import { translateServer } from '~/composables/servermsg'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { adminApi, ago, bytes } from '~/composables/admin'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t, num } from '~/composables/i18n'

type Ext = { id: number, mailbox: string, address: string, displayName: string, protocol: string, host: string, port: number, security: string,
  username: string, password?: string, smtpHost: string, smtpPort: number, smtpSecurity: string, smtpUsername: string, smtpPassword?: string,
  hasSmtpPassword?: boolean, inboxOnly: boolean, since: string, deleteAfter: boolean, enabled: boolean, status?: string, lastError?: string,
  lastRun?: string, imported?: number, progress?: string, interval?: number, keepDays: number, twoWay: boolean, remaining?: number, remainingBytes?: number }

type Preset = Partial<Ext> & { title: string, note?: string }
const PRESETS = computed<Record<string, Preset>>(() => ({
  gmail: { title: 'Gmail', host: 'imap.gmail.com', port: 993, security: 'ssl', smtpHost: 'smtp.gmail.com', smtpPort: 465, smtpSecurity: 'ssl',
    note: t('admin.collector.noteGmail') },
  yandex: { title: t('admin.collector.presetYandex'), host: 'imap.yandex.ru', port: 993, security: 'ssl', smtpHost: 'smtp.yandex.ru', smtpPort: 465, smtpSecurity: 'ssl',
    note: t('admin.collector.noteYandex') },
  mailru: { title: 'Mail.ru', host: 'imap.mail.ru', port: 993, security: 'ssl', smtpHost: 'smtp.mail.ru', smtpPort: 465, smtpSecurity: 'ssl',
    note: t('admin.collector.noteMailru') },
  regru: { title: t('admin.collector.presetRegru'), host: 'mail.hosting.reg.ru', port: 993, security: 'ssl', smtpHost: 'mail.hosting.reg.ru', smtpPort: 465, smtpSecurity: 'ssl',
    note: t('admin.collector.noteRegru') },
  other: { title: t('admin.collector.presetOther'), host: '', port: 993, security: 'ssl', smtpHost: '', smtpPort: 465, smtpSecurity: 'ssl' },
}))
const INTERVALS = computed(() => [1, 2, 5, 10, 15, 30].map(n => ({ value: n, label: t('admin.collector.everyMinutes', { n }) }))
  .concat([{ value: 60, label: t('admin.collector.hourly') }]))
const everyText = (n?: number) => INTERVALS.value.find(i => i.value === (n || 2))?.label || t('admin.collector.everyMinutes', { n })
const SEC = computed(() => [{ value: 'ssl', label: 'SSL/TLS' }, { value: 'starttls', label: 'STARTTLS' }, { value: 'none', label: t('admin.collector.secNone') }])
const PROTO = computed(() => [{ value: 'imap', label: t('admin.collector.protoImap') }, { value: 'pop3', label: t('admin.collector.protoPop3') }])

const list = ref<Ext[]>([])
const edit = ref<Ext | null>(null)
const preset = ref('other')
const test = ref<Record<string, string> | null>(null)
const busy = ref('')
let poll: ReturnType<typeof setInterval> | null = null

const loaded = ref(false)
async function load() {
  try { list.value = await adminApi<Ext[]>('/collector') } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { loaded.value = true }
}
onMounted(async () => {
  load()
  poll = setInterval(() => { if (list.value.some(a => a.progress || a.status === 'importing')) load() }, 3000)
})
onBeforeUnmount(() => { if (poll) clearInterval(poll) })

function create() {
  preset.value = 'other'
  test.value = null
  edit.value = { id: 0, mailbox: '', address: '', displayName: '', protocol: 'imap', host: '', port: 993, security: 'ssl',
    username: '', password: '', smtpHost: '', smtpPort: 465, smtpSecurity: 'ssl', smtpUsername: '', smtpPassword: '', inboxOnly: false, since: '', deleteAfter: false, enabled: true, interval: 2, keepDays: 90, twoWay: true }
}
function open(a: Ext) { preset.value = 'other'; test.value = null; edit.value = { ...a, password: '', smtpPassword: '' } }
function applyPreset(k: string) {
  preset.value = k
  const p = PRESETS.value[k]!
  if (edit.value) Object.assign(edit.value, { host: p.host, port: p.port, security: p.security, smtpHost: p.smtpHost, smtpPort: p.smtpPort, smtpSecurity: p.smtpSecurity, protocol: 'imap' })
}
function onAddress() {
  const e = edit.value
  if (!e) return
  if (!e.username || e.username === prevAddr) e.username = e.address
  prevAddr = e.address
  const d = e.address.split('@')[1] || ''
  if (!e.host) {
    if (d === 'gmail.com') applyPreset('gmail')
    else if (/^(yandex\.ru|ya\.ru)$/.test(d)) applyPreset('yandex')
    else if (/^(mail\.ru|bk\.ru|inbox\.ru|list\.ru)$/.test(d)) applyPreset('mailru')
  }
}
let prevAddr = ''
function onProto() { const e = edit.value; if (e) e.port = e.protocol === 'pop3' ? (e.security === 'ssl' ? 995 : 110) : (e.security === 'ssl' ? 993 : 143) }

async function runTest() {
  if (!edit.value) return
  busy.value = 'test'; test.value = null
  try { test.value = await adminApi('/collector/test', { method: 'POST', body: edit.value, query: { id: edit.value.id || '' }, timeout: 60000 }) }
  catch (e: any) { test.value = { mail: t('admin.collector.testError', { error: e.message }) } } finally { busy.value = '' }
}
async function save() {
  const e = edit.value!
  busy.value = 'save'
  try {
    const r = await adminApi<{ id: number }>(e.id ? `/collector/${e.id}` : '/collector', { method: e.id ? 'PUT' : 'POST', body: e })
    if (!e.id && e.enabled) await adminApi(`/collector/${r.id}/sync`, { method: 'POST' })
    toast(e.id ? t('common.saved') : t('admin.collector.connected'))
    edit.value = null
    load()
  } catch (x: any) { toast(x.message, { kind: 'err', ms: 8000 }) } finally { busy.value = '' }
}
async function remove(a: Ext) {
  if (!await confirmDialog(t('admin.collector.removeConfirm', { address: a.address }), t('admin.collector.removeConfirmText', { mailbox: a.mailbox }), { ok: t('admin.collector.disconnect'), danger: true })) return
  await adminApi(`/collector/${a.id}`, { method: 'DELETE' }); edit.value = null; load()
}
async function syncNow(a: Ext) { await adminApi(`/collector/${a.id}/sync`, { method: 'POST' }); toast(t('admin.collector.fetching')); setTimeout(load, 1500) }
const statusText = (a: Ext) => a.progress ? t('admin.collector.statusProgress', { progress: a.progress }) : a.status === 'error' ? t('common.error') : a.status === 'importing' ? t('admin.collector.statusImporting') : a.status === 'new' ? t('admin.collector.statusNew') : a.enabled ? t('admin.collector.statusWorking') : t('admin.collector.statusOff')
const note = computed(() => PRESETS.value[preset.value]?.note)
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="!loaded" :text="t('admin.collector.loading')" />
    <AdminHeader :title="t('admin.collector.title')" :desc="t('admin.collector.desc')">
      <button class="btn primary" @click="create"><AppIcon name="plus" />{{ t('admin.collector.connectMailbox') }}</button>
    </AdminHeader>

    <section v-if="!edit" class="card">
      <div v-if="!list.length" class="card-body hint">{{ t('admin.collector.empty') }}</div>
      <div v-for="a in list" :key="a.id" class="ext-row">
        <span class="st" :class="a.status === 'error' ? 'bad' : a.progress || a.status === 'importing' ? 'busy' : a.enabled ? 'ok' : ''"></span>
        <div class="ext-main">
          <b>{{ a.address }}</b> <span class="hint">{{ a.protocol.toUpperCase() }} {{ a.host }}<template v-if="a.smtpHost"> · {{ t('admin.collector.sendVia', { host: a.smtpHost }) }}</template><template v-else> · {{ t('admin.collector.readOnly') }}</template><template v-if="a.twoWay && a.protocol === 'imap'"> · {{ t('admin.collector.twoWayShort') }}</template><template v-if="a.deleteAfter"> · {{ a.keepDays ? t('admin.collector.onSourceDays', { n: a.keepDays }) : t('admin.collector.onSourceNone') }}</template></span>
          <span :class="['ext-status', { err: a.status === 'error' }]">{{ statusText(a) }}<template v-if="a.enabled"> · {{ everyText(a.interval) }}</template> · {{ t('admin.collector.imported', { n: num(a.imported || 0) }) }}<template v-if="a.remaining"> · {{ t('admin.collector.remaining', { n: num(a.remaining), size: bytes(a.remainingBytes || 0) }) }}</template><template v-if="a.lastRun"> · {{ t('admin.collector.checked', { when: ago(a.lastRun) }) }}</template></span>
          <span v-if="a.status === 'error'" class="ext-err">{{ translateServer(a.lastError || '') }}</span>
        </div>
        <button class="btn sm" :disabled="!!a.progress" @click="syncNow(a)"><AppIcon name="refresh" />{{ t('admin.collector.fetchNow') }}</button>
        <button class="btn sm" @click="open(a)"><AppIcon name="edit" />{{ t('admin.collector.configure') }}</button>
      </div>
    </section>

    <section v-else class="card">
      <div class="card-head"><h2>{{ edit.id ? edit.address : t('admin.collector.connectExternal') }}</h2></div>
      <form class="card-body form ext-form veil-host" @submit.prevent="save">
        <LoadingVeil :show="!!busy" :text="busy === 'test' ? t('admin.collector.testing') : t('common.saving')" />
        <div class="row2">
          <label class="f">{{ t('admin.collector.address') }}<input v-model="edit.address" type="email" placeholder="name@gmail.com" :disabled="!!edit.id" @change="onAddress" /></label>
          <label class="f">{{ t('admin.collector.displayName') }}<input v-model="edit.displayName" type="text" :placeholder="t('admin.collector.displayNamePlaceholder')" /></label>
        </div>
        <p v-if="!edit.id" class="hint" style="margin:-4px 0 0">{{ t('admin.collector.newHint') }}</p>
        <div class="presets"><span class="hint">{{ t('admin.collector.provider') }}</span>
          <button v-for="(p, k) in PRESETS" :key="k" type="button" class="chip" :class="{ on: preset === k }" @click="applyPreset(k as string)">{{ p.title }}</button></div>
        <p v-if="note" class="notice">{{ note }}</p>
        <label class="f">{{ t('admin.collector.protocol') }}<AppSelect v-model="edit.protocol" :options="PROTO" :aria-label="t('admin.collector.protocol')" @update:model-value="onProto" /></label>
        <div class="row3">
          <label class="f">{{ t('admin.collector.server', { proto: edit.protocol.toUpperCase() }) }}<input v-model="edit.host" type="text" placeholder="imap.example.ru" /></label>
          <label class="f">{{ t('admin.collector.port') }}<input v-model.number="edit.port" type="number" /></label>
          <label class="f">{{ t('admin.collector.security') }}<AppSelect v-model="edit.security" :options="SEC" :aria-label="t('admin.collector.security')" /></label>
        </div>
        <div class="row2">
          <label class="f">{{ t('common.login') }}<input v-model="edit.username" type="text" autocomplete="off" /></label>
          <label class="f">{{ t('common.password') }}<input v-model="edit.password" type="password" autocomplete="new-password" :placeholder="edit.id ? t('admin.collector.unchangedIfEmpty') : ''" /></label>
        </div>
        <details class="ext-smtp" open>
          <summary>{{ t('admin.collector.smtpTitle') }}</summary>
          <p class="hint">{{ edit.address ? t('admin.collector.smtpHint', { address: edit.address }) : t('admin.collector.smtpHintNoAddress') }}</p>
          <div class="row3">
            <label class="f">{{ t('admin.collector.server', { proto: 'SMTP' }) }}<input v-model="edit.smtpHost" type="text" placeholder="smtp.example.ru" /></label>
            <label class="f">{{ t('admin.collector.port') }}<input v-model.number="edit.smtpPort" type="number" /></label>
            <label class="f">{{ t('admin.collector.security') }}<AppSelect v-model="edit.smtpSecurity" :options="SEC" :aria-label="t('admin.collector.smtpSecurity')" /></label>
          </div>
          <div class="row2">
            <label class="f">{{ t('admin.collector.smtpLogin') }}<input v-model="edit.smtpUsername" type="text" :placeholder="t('admin.collector.sameAsMail')" /></label>
            <label class="f">{{ t('admin.collector.smtpPassword') }}<input v-model="edit.smtpPassword" type="password" autocomplete="new-password" :placeholder="edit.hasSmtpPassword ? t('admin.collector.unchangedIfEmpty') : t('admin.collector.sameAsMail')" /></label>
          </div>
        </details>
        <div class="row2">
          <label class="f">{{ t('admin.collector.since') }}<input v-model="edit.since" type="date" /></label>
          <div class="f checks">
            <label v-if="edit.protocol === 'imap'" class="check"><input v-model="edit.inboxOnly" type="checkbox" />{{ t('admin.collector.inboxOnly') }}</label>
            <label v-if="edit.protocol === 'imap'" class="check" :title="t('admin.collector.twoWayTitle')"><input v-model="edit.twoWay" type="checkbox" />{{ t('admin.collector.twoWay') }}</label>
            <label class="check"><input v-model="edit.deleteAfter" type="checkbox" />{{ t('admin.collector.deleteAfter') }}</label>
            <label v-if="edit.deleteAfter" class="interval">{{ t('admin.collector.keepBefore') }} <input v-model.number="edit.keepDays" type="number" min="0" max="3650" class="days" /> {{ t('admin.collector.keepAfter') }}</label>
            <span v-if="edit.deleteAfter" class="hint" style="padding-left:23px">{{ edit.keepDays ? t('admin.collector.deleteOlder', { n: edit.keepDays }) : t('admin.collector.deleteImmediately') }}{{ /gmail/.test(edit.host) ? ' ' + t('admin.collector.gmailTrash') : '' }}</span>
            <label class="check"><input v-model="edit.enabled" type="checkbox" />{{ t('admin.collector.auto') }}</label>
            <label v-if="edit.enabled" class="interval">{{ t('admin.collector.check') }} <AppSelect v-model="edit.interval" :options="INTERVALS" small :aria-label="t('admin.collector.checkAria')" /></label>
          </div>
        </div>
        <div v-if="test" class="notice">
          <div>{{ translateServer(test.mail) }}</div><div v-if="test.smtp">{{ translateServer(test.smtp) }}</div>
        </div>
        <div class="ext-actions">
          <button v-if="edit.id" type="button" class="btn danger-text" @click="remove(edit)">{{ t('admin.collector.disconnectCollector') }}</button>
          <NuxtLink v-if="edit.id" class="btn danger-text" :to="{ path: '/admin/mailboxes', query: { open: edit.mailbox } }"><AppIcon name="trash" />{{ t('admin.collector.deleteMailbox') }}</NuxtLink>
          <span class="grow"></span>
          <button type="button" class="btn" @click="edit = null">{{ t('common.cancel') }}</button>
          <button type="button" class="btn" :disabled="!!busy" @click="runTest">{{ busy === 'test' ? t('admin.collector.checking') : t('admin.collector.testConnection') }}</button>
          <button class="btn primary" :disabled="!!busy">{{ busy === 'save' ? t('common.saving') : edit.id ? t('common.save') : t('admin.collector.connectAndFetch') }}</button>
        </div>
      </form>
    </section>
  </div>
</template>

<style scoped>
.ext-row{display:flex;align-items:center;gap:12px;padding:12px 16px;border-bottom:1px solid var(--line)}
.ext-row:last-child{border-bottom:0}
.ext-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px}
.ext-status{font-size:13px;color:var(--ink-2)}
.ext-status.err,.ext-err{color:var(--danger)}
.ext-err{font-size:12.5px}
.st{width:10px;height:10px;border-radius:50%;background:var(--ink-3);flex:none}
.st.ok{background:#2e9d5b}.st.bad{background:var(--danger)}.st.busy{background:#e8a317;animation:vpulse 1s infinite}
.ext-form{display:flex;flex-direction:column;gap:14px}
.row3{display:grid;grid-template-columns:2fr 1fr 1.2fr;gap:12px}
.presets{display:flex;gap:6px;align-items:center;flex-wrap:wrap}
.ext-smtp summary{cursor:pointer;font-weight:600;color:var(--accent)}
.ext-smtp[open]{display:flex;flex-direction:column;gap:10px}
.checks{display:flex;flex-direction:column;gap:8px;justify-content:center}
.check{display:flex;align-items:center;gap:8px;font-size:14px}
.interval{display:flex;align-items:center;gap:8px;font-size:14px;padding-left:23px}
.days{width:70px;height:32px;border:1px solid var(--line);border-radius:4px;padding:0 8px;background:var(--surface-2);color:var(--ink);font:inherit}
.ext-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
@media (max-width:760px){.row3{grid-template-columns:1fr}.ext-row{flex-wrap:wrap}}
</style>
