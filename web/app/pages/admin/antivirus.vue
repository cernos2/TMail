<script setup lang="ts">
// Antivirus: ClamAV status, letters refused at SMTP time and the quarantine of collector imports.
import { translateServer } from '~/composables/servermsg'
import { ref, onMounted } from 'vue'
import { adminApi, dateTime } from '~/composables/admin'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { fileSize } from '~/composables/format'
import { t, fmtDate } from '~/composables/i18n'

type QItem = { id: number, mailbox: string, source: string, sender: string, subject: string, virus: string, size: number, created: string, released: string | null, releasedBy: string }
type Rejected = { time: string, sender: string, rcpt: string[], subject: string, reason: 'virus' | 'exec', virus: string }
type Status = { ok: boolean, error?: string, version?: string, engine?: string, signatures?: string, updated?: string, ageHours?: number, stale?: boolean, quarantine: QItem[], rejected: Rejected[] }

const s = ref<Status | null>(null)
const testing = ref(false)
async function load() { try { s.value = await adminApi<Status>('/antivirus') } catch (e: any) { toast(e.message, { kind: 'err' }) } }
onMounted(load)

async function test() {
  testing.value = true
  try {
    const r = await adminApi<{ detected: boolean, virus: string, ms: number }>('/antivirus/test', { method: 'POST' })
    toast(r.detected ? t('admin.antivirus.testOk', { ms: r.ms, virus: r.virus }) : t('admin.antivirus.testMissed'), { kind: r.detected ? 'info' : 'err', ms: 6000 })
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { testing.value = false }
}
async function act(q: QItem, action: 'release' | 'delete') {
  const ok = action === 'release'
    ? await confirmDialog(t('admin.antivirus.releaseConfirm'), t('admin.antivirus.releaseConfirmText', { mailbox: q.mailbox, virus: q.virus }), { ok: t('admin.antivirus.releaseOk'), danger: true })
    : await confirmDialog(t('admin.antivirus.deleteConfirm'), t('admin.antivirus.deleteConfirmText'), { ok: t('common.delete'), danger: true })
  if (!ok) return
  try { await adminApi(`/antivirus/quarantine/${q.id}?action=${action}`, { method: 'POST' }); toast(action === 'release' ? t('admin.antivirus.released') : t('admin.antivirus.deleted')); load() } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
const updated = (v?: string) => v ? fmtDate(v, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="!s || testing" :text="testing ? t('admin.antivirus.testing') : t('admin.antivirus.loading')" />
    <AdminHeader :title="t('admin.antivirus.title')" :desc="t('admin.antivirus.desc')" />

    <section class="card">
      <div class="card-head av-head"><h2>{{ t('admin.antivirus.status') }}</h2><span class="grow"></span>
        <button class="btn sm" :disabled="testing || !s?.ok" @click="test"><AppIcon name="shield" />{{ testing ? t('admin.antivirus.checking') : t('admin.antivirus.testButton') }}</button></div>
      <div v-if="s" class="kv">
        <div>{{ t('admin.antivirus.title') }}</div><div><span class="badge" :class="s.ok ? 'ok' : 'bad'">{{ s.ok ? t('admin.antivirus.running') : t('admin.antivirus.down') }}</span> {{ s.engine || translateServer(s.error || '') }}</div>
        <template v-if="s.ok"><div>{{ t('admin.antivirus.signatures') }}</div><div>{{ s.ageHours !== undefined ? t('admin.antivirus.signaturesInfoAge', { version: s.signatures, date: updated(s.updated), hours: s.ageHours }) : t('admin.antivirus.signaturesInfo', { version: s.signatures, date: updated(s.updated) }) }}
          <span v-if="s.stale" class="badge bad">{{ t('admin.antivirus.stale') }}</span></div></template>
        <div>{{ t('admin.antivirus.whatScanned') }}</div><div>{{ t('admin.antivirus.whatScannedText') }}</div>
        <div>{{ t('admin.antivirus.extra') }}</div><div>{{ t('admin.antivirus.extraText') }}</div>
      </div>
      <div v-else class="empty-card" style="min-height:240px"></div>
    </section>

    <section class="card">
      <div class="card-head"><h2>{{ t('admin.antivirus.quarantine') }}</h2><span class="hint">{{ t('admin.antivirus.quarantineHint') }}</span></div>
      <div v-for="q in s?.quarantine || []" :key="q.id" class="av-item">
        <AppIcon name="shieldX" class="av-ico" />
        <div class="av-main">
          <b>{{ q.subject || t('common.noSubject') }}</b>
          <span class="hint">{{ q.sender }} → {{ q.mailbox }} · {{ dateTime(q.created) }} · {{ fileSize(q.size) }}</span>
          <span class="av-virus">{{ q.virus }}</span>
        </div>
        <span v-if="q.released" class="hint">{{ t('admin.antivirus.releasedBy', { by: q.releasedBy, date: dateTime(q.released) }) }}</span>
        <template v-else>
          <button class="link" @click="act(q, 'release')">{{ t('admin.antivirus.release') }}</button>
          <button class="link danger-text" @click="act(q, 'delete')">{{ t('admin.antivirus.delete') }}</button>
        </template>
      </div>
      <p v-if="s && !s.quarantine.length" class="card-body hint">{{ t('admin.antivirus.quarantineEmpty') }}</p>
    </section>

    <section class="card">
      <div class="card-head"><h2>{{ t('admin.antivirus.rejected') }}</h2><span class="hint">{{ t('admin.antivirus.rejectedHint') }}</span></div>
      <div v-for="(r, i) in s?.rejected || []" :key="i" class="av-item">
        <AppIcon :name="r.reason === 'virus' ? 'shieldX' : 'alert'" class="av-ico" />
        <div class="av-main">
          <b>{{ r.subject || t('common.noSubject') }}</b>
          <span class="hint">{{ r.sender }} → {{ r.rcpt.join(', ') }} · {{ dateTime(r.time) }}</span>
          <span class="av-virus">{{ r.reason === 'virus' ? t('admin.antivirus.reasonVirus', { name: r.virus }) : t('admin.antivirus.reasonExec', { name: r.virus }) }}</span>
        </div>
      </div>
      <p v-if="s && !s.rejected.length" class="card-body hint">{{ t('admin.antivirus.rejectedEmpty') }}</p>
    </section>
  </div>
</template>

<style scoped>
.av-head{display:flex;align-items:center;gap:10px}
.av-item{display:flex;align-items:center;gap:12px;padding:10px 16px;border-top:1px solid var(--line)}
.av-ico{width:20px;height:20px;color:var(--danger);flex:none}
.av-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px;font-size:14px}
.av-main b{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.av-virus{font-family:var(--f-mono);font-size:12px;color:var(--danger)}
.badge.ok{color:var(--ok, #1E8048)}
.badge.bad{color:var(--danger)}
</style>
