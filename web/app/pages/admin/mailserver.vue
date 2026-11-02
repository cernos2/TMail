<script setup lang="ts">
// Mail server settings: delivery protection (MTA-STS, DANE), sending through an external SMTP
// server, sending limits per mailbox, and moving the whole configuration to another server.
import { ref, reactive, onMounted } from 'vue'
import { adminApi } from '~/composables/admin'
import { toast } from '~/composables/toast'
import { confirmDialog } from '~/composables/dialogs'
import { downloadBlob } from '~/composables/mailclient'
import { mail } from '~/composables/mail'
import { t, fmtDate, num } from '~/composables/i18n'

type Settings = { mtaStsMode: string, mtaStsId: string, limitHour: number, limitDay: number,
  relay: { enabled: boolean, host: string, port: number, security: string, username: string, hasPassword: boolean, status: string } }
const s = ref<Settings | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const relay = reactive({ enabled: false, host: '', port: 587, security: 'starttls', username: '', password: '' })
const limits = reactive({ hour: 200, day: 2000 })
const relayTest = ref<{ ok: boolean, error?: string } | null>(null)
const testing = ref(false)

async function load() {
  loading.value = true
  try {
    s.value = await adminApi<Settings>('/mail-settings')
    Object.assign(relay, { enabled: s.value.relay.enabled, host: s.value.relay.host, port: s.value.relay.port || 587, security: s.value.relay.security || 'starttls', username: s.value.relay.username, password: '' })
    Object.assign(limits, { hour: s.value.limitHour, day: s.value.limitDay })
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

async function save(body: Record<string, any>, done = t('common.saved')) {
  saving.value = true
  try { s.value = await adminApi<Settings>('/mail-settings', { method: 'PUT', body }); toast(done) }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { saving.value = false }
}
function setMode(m: string) { save({ mtaStsMode: m }) }
function saveLimits() { save({ limitHour: Math.max(0, Math.round(limits.hour || 0)), limitDay: Math.max(0, Math.round(limits.day || 0)) }) }
async function saveRelay() {
  if (relay.enabled && !relay.host.trim()) { toast(t('admin.mailserver.relayHostRequired'), { kind: 'err' }); return }
  await save({ relay: { enabled: relay.enabled, host: relay.host.trim(), port: Number(relay.port), security: relay.security, username: relay.username.trim(),
    ...(relay.password ? { password: relay.password } : {}) } }, relay.enabled ? t('admin.mailserver.relayOn') : t('admin.mailserver.relayOff'))
  relay.password = ''
}
async function testRelay() {
  testing.value = true
  relayTest.value = null
  try {
    relayTest.value = await adminApi('/mail-settings/relay-test', { method: 'POST', body: { host: relay.host.trim(), port: Number(relay.port), security: relay.security,
      username: relay.username.trim(), ...(relay.password ? { password: relay.password } : {}) } })
  } catch (e: any) { relayTest.value = { ok: false, error: e.message } } finally { testing.value = false }
}
function onPort() { relay.security = Number(relay.port) === 465 ? 'ssl' : 'starttls' }

// ---------- configuration transfer ----------
const exp = reactive({ pass: '', pass2: '', history: false, busy: false })
async function doExport() {
  if (exp.pass.length < 10) { toast(t('admin.mailserver.passShort'), { kind: 'err' }); return }
  if (exp.pass !== exp.pass2) { toast(t('admin.mailserver.passMismatch'), { kind: 'err' }); return }
  exp.busy = true
  try {
    const r = await fetch('/api/admin/config/export', { method: 'POST', credentials: 'same-origin', headers: { 'X-ZM': '1', 'Content-Type': 'application/json' },
      body: JSON.stringify({ passphrase: exp.pass, collectorHistory: exp.history }) })
    if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error || t('admin.mailserver.exportFailed'))
    const name = /filename="([^"]+)"/.exec(r.headers.get('Content-Disposition') || '')?.[1] || 'tmail-config.tmailcfg'
    downloadBlob(await r.blob(), name)
    toast(t('admin.mailserver.exported'))
    exp.pass = ''; exp.pass2 = ''
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { exp.busy = false }
}
const imp = reactive({ file: null as File | null, pass: '', busy: false, summary: null as any, confirm: '' })
const impInput = ref<HTMLInputElement | null>(null)
async function importStep(mode: 'check' | 'apply') {
  if (!imp.file || !imp.pass) { toast(t('admin.mailserver.importNeed'), { kind: 'err' }); return }
  if (mode === 'apply' && !await confirmDialog(t('admin.mailserver.importConfirm'), t('admin.mailserver.importConfirmText'), { ok: t('admin.mailserver.importApply'), danger: true })) return
  imp.busy = true
  try {
    const fd = new FormData()
    fd.append('file', imp.file); fd.append('passphrase', imp.pass); fd.append('mode', mode)
    const r = await fetch('/api/admin/config/import', { method: 'POST', body: fd, credentials: 'same-origin', headers: { 'X-ZM': '1' } })
    const j = await r.json().catch(() => ({}))
    if (!r.ok) throw new Error(j.error || t('admin.mailserver.importFailed'))
    if (mode === 'check') { imp.summary = j; return }
    toast(t('admin.mailserver.imported'), { ms: 15000 })
    setTimeout(() => { mail.me = null; location.href = '/login' }, 2500)
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { imp.busy = false }
}
const countRows = (c: Record<string, number>) => ['domains', 'mailboxes', 'aliases', 'users', 'calendars', 'cal_events', 'crm_parties', 'external_accounts', 'reply_templates']
  .filter(k => c[k] != null).map(k => [t(`admin.mailserver.table.${k}`), num(c[k])])
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.mailserver.loading')" />
    <AdminHeader :title="t('admin.mailserver.title')" :desc="t('admin.mailserver.desc')" />
    <div v-if="error" class="notice danger">{{ error }}</div>
    <template v-if="s">
      <section class="card veil-host">
        <LoadingVeil :show="saving" :text="t('common.saving')" />
        <div class="card-head"><h2><AppIcon name="shield" />{{ t('admin.mailserver.protection') }}</h2></div>
        <div class="card-body ms-body form">
          <div class="ms-row">
            <div><b>{{ t('admin.mailserver.mtaSts') }}</b><span class="hint">{{ t('admin.mailserver.mtaStsHint') }}</span></div>
            <span class="seg" role="group" :aria-label="t('admin.mailserver.mtaSts')">
              <button :class="{ on: s.mtaStsMode === 'testing' }" @click="setMode('testing')">{{ t('admin.mailserver.modeTesting') }}</button>
              <button :class="{ on: s.mtaStsMode === 'enforce' }" @click="setMode('enforce')">{{ t('admin.mailserver.modeEnforce') }}</button>
              <button :class="{ on: s.mtaStsMode === 'none' }" @click="setMode('none')">{{ t('admin.mailserver.modeNone') }}</button>
            </span>
          </div>
          <p class="hint ms-note">{{ s.mtaStsMode === 'enforce' ? t('admin.mailserver.modeEnforceText') : s.mtaStsMode === 'testing' ? t('admin.mailserver.modeTestingText') : t('admin.mailserver.modeNoneText') }}
            <NuxtLink to="/admin/domains" class="link">{{ t('admin.mailserver.dnsRecords') }}</NuxtLink></p>
          <div class="ms-row">
            <div><b>{{ t('admin.mailserver.outgoing') }}</b><span class="hint">{{ t('admin.mailserver.outgoingHint') }}</span></div>
            <span class="pill ok"><AppIcon name="check" />{{ t('admin.mailserver.alwaysOn') }}</span>
          </div>
        </div>
      </section>

      <section class="card veil-host">
        <LoadingVeil :show="testing" :text="t('admin.mailserver.relayTesting')" />
        <div class="card-head"><h2><AppIcon name="sent" />{{ t('admin.mailserver.relay') }}</h2>
          <span v-if="s.relay.status" class="hint" style="margin-left:auto">{{ s.relay.status.startsWith('on') ? t('admin.mailserver.relayApplied', { host: s.relay.status.split(' ')[1] }) : t('admin.mailserver.relayDirect') }}</span></div>
        <div class="card-body ms-body form">
          <p class="hint" style="margin:0">{{ t('admin.mailserver.relayHint') }}</p>
          <label class="check"><input v-model="relay.enabled" type="checkbox" />{{ t('admin.mailserver.relayEnable') }}</label>
          <div class="ms-grid" :class="{ off: !relay.enabled }">
            <label class="f">{{ t('admin.mailserver.relayHost') }}<input v-model="relay.host" type="text" placeholder="smtp.example.com" autocapitalize="none" spellcheck="false" /></label>
            <label class="f">{{ t('admin.mailserver.relayPort') }}<input v-model.number="relay.port" type="number" min="1" max="65535" @change="onPort" /></label>
            <div class="f"><span>{{ t('admin.mailserver.relaySecurity') }}</span>
              <span class="seg"><button type="button" :class="{ on: relay.security === 'starttls' }" @click="relay.security = 'starttls'">STARTTLS</button>
                <button type="button" :class="{ on: relay.security === 'ssl' }" @click="relay.security = 'ssl'">SSL/TLS</button></span></div>
            <label class="f">{{ t('admin.mailserver.relayLogin') }}<input v-model="relay.username" type="text" autocomplete="off" autocapitalize="none" spellcheck="false" /></label>
            <label class="f">{{ t('common.password') }}<input v-model="relay.password" type="password" autocomplete="new-password" :placeholder="s.relay.hasPassword ? t('admin.mailserver.passwordKept') : ''" /></label>
          </div>
          <div v-if="relayTest" class="notice" :class="relayTest.ok ? 'ok' : 'danger'"><AppIcon :name="relayTest.ok ? 'check' : 'alert'" />{{ relayTest.ok ? t('admin.mailserver.relayTestOk') : relayTest.error }}</div>
          <div class="ms-foot">
            <button class="btn" :disabled="!relay.host.trim() || testing" @click="testRelay"><AppIcon name="refresh" />{{ t('admin.mailserver.relayTest') }}</button>
            <button class="btn primary" :disabled="saving" @click="saveRelay">{{ t('common.save') }}</button>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-head"><h2><AppIcon name="alert" />{{ t('admin.mailserver.limits') }}</h2></div>
        <div class="card-body ms-body form">
          <p class="hint" style="margin:0">{{ t('admin.mailserver.limitsHint') }}</p>
          <div class="ms-grid two">
            <label class="f">{{ t('admin.mailboxes.limitHour') }}<input v-model.number="limits.hour" type="number" min="0" step="1" /></label>
            <label class="f">{{ t('admin.mailboxes.limitDay') }}<input v-model.number="limits.day" type="number" min="0" step="1" /></label>
          </div>
          <span class="hint">{{ t('admin.mailserver.limitsZero') }}</span>
          <div class="ms-foot"><button class="btn primary" :disabled="saving" @click="saveLimits">{{ t('common.save') }}</button></div>
        </div>
      </section>

      <section class="card veil-host">
        <LoadingVeil :show="exp.busy || imp.busy" :text="exp.busy ? t('admin.mailserver.exporting') : t('admin.mailserver.importing')" />
        <div class="card-head"><h2><AppIcon name="server" />{{ t('admin.mailserver.transfer') }}</h2></div>
        <div class="card-body ms-body form">
          <p class="hint" style="margin:0">{{ t('admin.mailserver.transferHint') }}</p>
          <div class="ms-cols">
            <div class="ms-col form">
              <h3><AppIcon name="download" />{{ t('admin.mailserver.export') }}</h3>
              <label class="f">{{ t('admin.mailserver.passphrase') }}<input v-model="exp.pass" type="password" autocomplete="new-password" /></label>
              <label class="f">{{ t('admin.mailserver.passphrase2') }}<input v-model="exp.pass2" type="password" autocomplete="new-password" /></label>
              <label class="check"><input v-model="exp.history" type="checkbox" />{{ t('admin.mailserver.withHistory') }}</label>
              <span class="hint">{{ t('admin.mailserver.passHint') }}</span>
              <div class="ms-foot"><button class="btn primary" :disabled="exp.busy" @click="doExport"><AppIcon name="download" />{{ t('admin.mailserver.exportBtn') }}</button></div>
            </div>
            <div class="ms-col form">
              <h3><AppIcon name="upload" />{{ t('admin.mailserver.import') }}</h3>
              <div class="f"><span>{{ t('admin.mailserver.file') }}</span>
                <button type="button" class="btn" @click="impInput?.click()"><AppIcon name="file" />{{ imp.file ? imp.file.name : t('admin.mailserver.chooseFile') }}</button>
                <input ref="impInput" type="file" accept=".tmailcfg" hidden @change="imp.file = ($event.target as HTMLInputElement).files?.[0] || null; imp.summary = null" /></div>
              <label class="f">{{ t('admin.mailserver.passphrase') }}<input v-model="imp.pass" type="password" autocomplete="off" @input="imp.summary = null" /></label>
              <div v-if="imp.summary" class="ms-summary">
                <b>{{ t('admin.mailserver.fileFrom', { host: imp.summary.hostname, date: fmtDate(imp.summary.created, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }) }) }}</b>
                <div class="kv"><template v-for="r in countRows(imp.summary.counts || {})" :key="r[0]"><div>{{ r[0] }}</div><div>{{ r[1] }}</div></template></div>
                <span class="hint warn-text">{{ t('admin.mailserver.importWarn') }}</span>
              </div>
              <div class="ms-foot">
                <button v-if="!imp.summary" class="btn" :disabled="imp.busy || !imp.file" @click="importStep('check')">{{ t('admin.mailserver.importCheck') }}</button>
                <button v-else class="btn danger" :disabled="imp.busy" @click="importStep('apply')"><AppIcon name="upload" />{{ t('admin.mailserver.importApply') }}</button>
              </div>
            </div>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.card-head h2{display:flex;align-items:center;gap:8px}
.card-head h2 svg{width:18px;height:18px;color:var(--accent)}
.ms-body{display:flex;flex-direction:column;gap:12px}
.ms-body.form{gap:12px}
.ms-row{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap}
.ms-row > div{display:flex;flex-direction:column;gap:2px;max-width:620px}
.ms-row .hint{font-size:12.5px}
.ms-note{margin:-4px 0 6px;font-size:12.5px}
.ms-grid{display:grid;grid-template-columns:2fr 1fr 1fr;gap:10px;align-items:end}
.ms-grid .seg{height:38px}
.ms-grid .seg button{height:36px}
.ms-grid.two{grid-template-columns:1fr 1fr;max-width:520px}
.ms-grid.off{opacity:.55}
.ms-foot{display:flex;justify-content:flex-end;gap:8px}
.ms-cols{display:grid;grid-template-columns:1fr 1fr;gap:18px}
.ms-col{display:flex;flex-direction:column;gap:10px;padding:14px;border:1px solid var(--line);border-radius:6px;background:var(--surface-2)}
.ms-col h3{display:flex;align-items:center;gap:8px;margin:0;font-size:14px}
.ms-col h3 svg{width:16px;height:16px;color:var(--accent)}
.ms-col .hint{font-size:12.5px}
.ms-summary{display:flex;flex-direction:column;gap:8px;padding:10px 12px;border-radius:5px;background:var(--surface);border:1px solid var(--line)}
.notice.ok{background:var(--ok-soft);border-color:transparent;color:var(--ok)}
@media (max-width:900px){.ms-grid,.ms-cols{grid-template-columns:1fr}}
</style>
