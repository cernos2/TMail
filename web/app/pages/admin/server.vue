<script setup lang="ts">
import { translateServer } from '~/composables/servermsg'
import { ref, computed, onMounted } from 'vue'
import { adminApi, bytes } from '~/composables/admin'
import { t, fmtDate } from '~/composables/i18n'

const s = ref<any>(null)
const error = ref('')
const loading = ref(false)
async function load() {
  loading.value = true; error.value = ''
  try { s.value = await adminApi('/server') } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

const memUsed = computed(() => s.value?.memory ? s.value.memory.total - s.value.memory.available : 0)
const pct = (used: number, total: number) => total ? Math.round(used / total * 100) : 0
function uptime(sec: number) {
  const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600)
  return d ? t('admin.server.uptimeDays', { d, h }) : t('admin.server.uptimeHours', { h, m: Math.floor(sec % 3600 / 60) })
}
// Spam filter actions as shown to the administrator.
const ACT = computed<Record<string, string>>(() => ({ 'no action': t('admin.server.spamClean'), 'add header': t('admin.server.spamToSpam'), 'reject': t('admin.server.spamRejected'), 'greylist': t('admin.server.spamDeferred'), 'soft reject': t('admin.server.spamDeferred'), 'rewrite subject': t('admin.server.spamTagged') }))
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.server.checkingServer')" />
    <AdminHeader :title="t('admin.server.title')" :desc="s ? `${s.hostname} · ${s.publicIp || ''}` : ''">
      <button class="btn" :disabled="loading" @click="load"><AppIcon name="refresh" />{{ loading ? t('admin.server.checking') : t('common.refresh') }}</button>
    </AdminHeader>
    <div v-if="error" class="notice danger">{{ error }}</div>
    <template v-if="s">
      <section class="card">
        <div class="card-head"><h2>{{ t('admin.server.services') }}</h2></div>
        <div class="svc">
          <div v-for="c in s.services" :key="c.key" class="svc-item"><span class="st" :class="c.ok ? 'ok' : 'bad'"></span><div><b>{{ translateServer(c.title) }}</b><span>{{ translateServer(c.detail || '') }}</span></div></div>
        </div>
      </section>

      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:18px">
        <section class="card">
          <div class="card-head"><h2>{{ t('admin.server.resources') }}</h2></div>
          <div class="card-body" style="display:flex;flex-direction:column;gap:14px">
            <div v-if="s.cpu?.percent !== undefined" class="usage"><span>{{ t('admin.server.cpu', { percent: s.cpu.percent, cores: s.cpu.cores }) }}<template v-if="s.cpu.iowait >= 10"> · {{ t('admin.server.cpuIowait', { n: s.cpu.iowait }) }}</template></span>
              <div class="meter" :class="{ hot: s.cpu.percent > 70, full: s.cpu.percent > 90 }"><i :style="{ width: s.cpu.percent + '%' }"></i></div></div>
            <div class="usage"><span>{{ t('admin.server.memory', { used: bytes(memUsed), total: bytes(s.memory.total) }) }}</span>
              <div class="meter" :class="{ hot: pct(memUsed, s.memory.total) > 80, full: pct(memUsed, s.memory.total) > 92 }"><i :style="{ width: pct(memUsed, s.memory.total) + '%' }"></i></div></div>
            <div v-if="s.memory.swapTotal" class="usage"><span>{{ t('admin.server.swap', { used: bytes(s.memory.swapTotal - s.memory.swapFree), total: bytes(s.memory.swapTotal) }) }}</span>
              <div class="meter"><i :style="{ width: pct(s.memory.swapTotal - s.memory.swapFree, s.memory.swapTotal) + '%' }"></i></div></div>
            <div v-if="s.disk" class="usage"><span>{{ t('admin.server.disk', { free: bytes(s.disk.free), total: bytes(s.disk.total) }) }}</span>
              <div class="meter" :class="{ hot: pct(s.disk.total - s.disk.free, s.disk.total) > 85 }"><i :style="{ width: pct(s.disk.total - s.disk.free, s.disk.total) + '%' }"></i></div></div>
            <div class="hint">{{ s.cpu?.cores ? t('admin.server.loadAvgNorm', { load: s.load?.split(' ').join(' / '), cores: s.cpu.cores }) : t('admin.server.loadAvg', { load: s.load?.split(' ').join(' / ') }) }} · {{ t('admin.server.uptime', { uptime: uptime(s.uptimeSeconds || 0) }) }}</div>
            <div v-if="s.memory.total && s.memory.available / s.memory.total < 0.12" class="hint warn-text">{{ t('admin.server.lowMemory') }}</div>
          </div>
        </section>

        <section v-if="s.antivirus" class="card">
          <div class="card-head"><h2>{{ t('admin.server.antivirus') }}</h2><NuxtLink to="/admin/antivirus" class="link" style="margin-left:auto">{{ t('admin.server.details') }}</NuxtLink></div>
          <div class="kv">
            <div>{{ t('admin.server.avStatus') }}</div><div><span class="badge" :class="s.antivirus.ok ? 'ok' : 'bad'">{{ s.antivirus.ok ? t('admin.server.avRunning') : t('admin.server.avDown') }}</span> {{ s.antivirus.engine }}</div>
            <template v-if="s.antivirus.signatures"><div>{{ t('admin.server.avSignatures') }}</div><div>{{ t('admin.server.avSignaturesInfo', { n: s.antivirus.signatures, hours: s.antivirus.ageHours }) }}
              <span v-if="s.antivirus.ageHours > 48" class="badge bad">{{ t('admin.server.avOutdated') }}</span></div></template>
            <div>{{ t('admin.server.avFilesChecked') }}</div><div>{{ s.antivirus.found24h ? t('admin.server.avChecked24hFound', { n: s.antivirus.checked24h, found: s.antivirus.found24h }) : t('admin.server.avChecked24h', { n: s.antivirus.checked24h }) }}</div>
            <div>{{ t('admin.server.avLettersRejected') }}</div><div>{{ t('admin.server.avRejected24h', { n: s.antivirus.rejected24h }) }}</div>
            <div>{{ t('admin.server.avQuarantine') }}</div><div>{{ t('admin.server.avQuarantineCount', { n: s.antivirus.quarantine }) }}</div>
            <div v-if="s.antivirus.pendingLinks">{{ t('admin.server.avPending') }}</div><div v-if="s.antivirus.pendingLinks">{{ t('admin.server.avPendingLinks', { n: s.antivirus.pendingLinks }) }}</div>
          </div>
        </section>

        <section class="card">
          <div class="card-head"><h2>{{ t('admin.server.mail') }}</h2></div>
          <div class="kv">
            <div>{{ t('admin.server.queue') }}</div><div>{{ t('admin.server.queueInfo', { active: s.queue.active, deferred: s.queue.deferred }) }}</div>
            <template v-if="s.spam"><div>{{ t('admin.server.spamScanned') }}</div><div>{{ t('admin.server.spamScannedCount', { n: s.spam.scanned }) }}</div>
              <div>{{ t('admin.server.spamOfThem') }}</div><div>{{ Object.entries(s.spam.actions || {}).filter(([, v]) => v).map(([k, v]) => `${ACT[k] || k}: ${v}`).join(', ') || '—' }}</div></template>
            <template v-if="s.certificate"><div>{{ t('admin.server.certificate') }}</div><div>{{ t('admin.server.certificateInfo', { issuer: s.certificate.issuer, days: s.certificate.daysLeft, date: fmtDate(s.certificate.notAfter, { day: 'numeric', month: 'numeric', year: 'numeric' }) }) }}</div></template>
          </div>
        </section>
      </div>

      <section class="card">
        <div class="card-head"><h2>{{ t('admin.server.clientsTitle') }}</h2></div>
        <div class="kv">
          <div>{{ t('admin.server.imap') }}</div><div>{{ t('admin.server.clientHost', { host: s.clients.imap.host, port: s.clients.imap.port, security: s.clients.imap.security }) }}</div>
          <div>{{ t('admin.server.pop3') }}</div><div>{{ t('admin.server.clientHost', { host: s.clients.pop3.host, port: s.clients.pop3.port, security: s.clients.pop3.security }) }}</div>
          <div>{{ t('admin.server.smtp') }}</div><div>{{ t('admin.server.smtpHost', { host: s.clients.smtp.host, port: s.clients.smtp.port, security: s.clients.smtp.security, alt: s.clients.smtp.alt }) }}</div>
          <div>{{ t('admin.server.credentials') }}</div><div>{{ t('admin.server.credentialsHint') }}</div>
        </div>
      </section>
    </template>
    <div v-else-if="!error" class="card"><div class="empty-card" style="min-height:240px"></div></div>
  </div>
</template>

<style scoped>
.warn-text{color:#B26A00}
</style>
