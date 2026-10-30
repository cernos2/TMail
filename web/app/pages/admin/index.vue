<script setup lang="ts">
import { translateServer } from '~/composables/servermsg'
import { ref, computed, onMounted } from 'vue'
import { adminApi, dateTime, bytes, actionLabel } from '~/composables/admin'
import { t } from '~/composables/i18n'

type Check = { key: string, title: string, status: string, found: string, hint: string }
const data = ref<any>(null)
const error = ref('')
const loading = ref(true)

async function load() {
  loading.value = true; error.value = ''
  try { data.value = await adminApi('/overview') } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

const NAMES: Record<string, string> = { mx: 'MX', spf: 'SPF', dkim: 'DKIM', dmarc: 'DMARC', ptr: 'PTR' }
const recName = (k: string) => k === 'a' ? t('admin.overview.aRecord') : NAMES[k] || k


const alerts = computed(() => {
  const d = data.value
  if (!d) return []
  const out: { kind: 'bad' | 'warn', title: string, text: string, to?: string }[] = []
  for (const s of d.services || []) if (!s.ok) out.push({ kind: 'bad', title: t('admin.overview.serviceDown', { service: translateServer(s.title) }), text: t('admin.overview.serviceDownText'), to: '/admin/server' })
  for (const dom of d.domains || []) {
    const bad = (dom.checks as Check[]).filter(c => c.status !== 'ok' && c.key !== 'ptr' && c.key !== 'a')
    if (bad.length) out.push({ kind: 'bad', title: t('admin.overview.dnsIncomplete', { domain: dom.name }), text: t('admin.overview.dnsIncompleteText', { records: bad.map(c => recName(c.key)).join(', ') }), to: `/admin/domains?d=${dom.id}` })
  }
  const any = (d.domains || [])[0]
  const ptr = any?.checks?.find((c: Check) => c.key === 'ptr')
  if (ptr && ptr.status !== 'ok') out.push({ kind: 'warn', title: t('admin.overview.ptrMismatch'), text: t('admin.overview.ptrText', { found: ptr.found || t('admin.overview.noRecord'), hint: translateServer(ptr.hint || '') }), to: `/admin/domains?d=${any.id}` })
  const aRec = any?.checks?.find((c: Check) => c.key === 'a')
  if (aRec && aRec.status !== 'ok') out.push({ kind: 'bad', title: t('admin.overview.aMismatch'), text: translateServer(aRec.hint || ''), to: `/admin/domains?d=${any.id}` })
  const cert = d.server?.certificate
  if (cert && cert.daysLeft < 14) out.push({ kind: cert.daysLeft < 3 ? 'bad' : 'warn', title: t('admin.overview.certExpires', { days: cert.daysLeft }), text: t('admin.overview.certExpiresText'), to: '/admin/server' })
  const q = d.server?.queue
  if (q?.deferred > 0) out.push({ kind: 'warn', title: t('admin.overview.deferred', { n: q.deferred }), text: t('admin.overview.deferredText'), to: '/admin/server' })
  const m = d.server?.memory
  if (m?.total && m.available / m.total < 0.12) out.push({ kind: 'warn', title: t('admin.overview.lowMemory'), text: t('admin.overview.lowMemoryText', { free: bytes(m.available), total: bytes(m.total) }), to: '/admin/server' })
  const disk = d.server?.disk
  if (disk?.total && disk.free / disk.total < 0.1) out.push({ kind: 'warn', title: t('admin.overview.lowDisk'), text: t('admin.overview.lowDiskText', { free: bytes(disk.free) }), to: '/admin/server' })
  if (d.counts?.failedLogins24h >= 20) out.push({ kind: 'warn', title: t('admin.overview.failedLogins', { n: d.counts.failedLogins24h }), text: t('admin.overview.failedLoginsText'), to: '/admin/audit?kind=login' })
  return out
})

// label is a message key, translated at render time.
const STATS: { key: string, label: string, to: string }[] = [
  { key: 'users', label: 'admin.overview.statUsers', to: '/admin/users' },
  { key: 'mailboxes', label: 'admin.overview.statMailboxes', to: '/admin/mailboxes' },
  { key: 'aliases', label: 'admin.overview.statAliases', to: '/admin/aliases' },
  { key: 'domains', label: 'admin.overview.statDomains', to: '/admin/domains' },
  { key: 'sent24h', label: 'admin.overview.statSent24h', to: '/admin/audit?kind=sent' },
]
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.overview.checking')" />
    <AdminHeader :title="t('admin.overview.title')" :desc="t('admin.overview.desc')">
      <button class="btn" :disabled="loading" @click="load"><AppIcon name="refresh" />{{ loading ? t('admin.overview.checkingShort') : t('common.refresh') }}</button>
    </AdminHeader>

    <div v-if="error" class="notice danger">{{ error }}</div>
    <template v-else-if="data">
      <section class="card">
        <div class="card-head"><h2>{{ t('admin.overview.attention') }}</h2><span v-if="alerts.length" class="pill bad">{{ alerts.length }}</span></div>
        <div class="alerts">
          <NuxtLink v-for="a in alerts" :key="a.title" :to="a.to || ''" class="alert" :class="a.kind" style="text-decoration:none;color:inherit">
            <span class="ico"><AppIcon :name="a.kind === 'bad' ? 'alert' : 'junk'" /></span>
            <span class="txt"><b>{{ a.title }}</b><span>{{ a.text }}</span></span>
            <AppIcon name="chev" style="transform:rotate(-90deg);color:var(--ink-3)" />
          </NuxtLink>
          <div v-if="!alerts.length" class="alert ok">
            <span class="ico"><AppIcon name="check" /></span>
            <span class="txt"><b>{{ t('admin.overview.allGood') }}</b><span>{{ t('admin.overview.allGoodText') }}</span></span>
          </div>
        </div>
      </section>

      <div class="stats">
        <NuxtLink v-for="s in STATS" :key="s.key" :to="s.to" class="stat"><b>{{ data.counts[s.key] ?? 0 }}</b><span>{{ t(s.label, { n: data.counts[s.key] ?? 0 }) }}</span></NuxtLink>
      </div>

      <section class="card">
        <div class="card-head"><h2>{{ t('admin.overview.services') }}</h2><NuxtLink to="/admin/server" class="btn sm">{{ t('admin.overview.details') }}</NuxtLink></div>
        <div class="svc">
          <div v-for="s in data.services" :key="s.key" class="svc-item">
            <span class="st" :class="s.ok ? 'ok' : 'bad'"></span>
            <div><b>{{ translateServer(s.title) }}</b><span>{{ translateServer(s.detail || '') }}</span></div>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-head"><h2>{{ t('admin.overview.recent') }}</h2><NuxtLink to="/admin/audit" class="btn sm">{{ t('admin.overview.fullLog') }}</NuxtLink></div>
        <div class="tbl-wrap">
          <table class="tbl">
            <tbody>
              <tr v-for="e in data.recent" :key="e.id">
                <td class="num" style="text-align:left;width:150px">{{ dateTime(e.at) }}</td>
                <td><span :class="e.action === 'login.failed' ? 'pill bad' : 'pill'">{{ actionLabel(e.action) }}</span></td>
                <td>{{ e.target }}</td>
                <td class="muted">{{ e.actor }}</td>
              </tr>
              <tr v-if="!data.recent.length"><td class="empty-card">{{ t('admin.overview.noEvents') }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
    <div v-else class="card"><div class="empty-card" style="min-height:240px"></div></div>
  </div>
</template>
