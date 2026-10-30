<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { adminApi, dateTime, actionLabel } from '~/composables/admin'
import { t } from '~/composables/i18n'

const route = useRoute()
const kind = ref<string>((route.query.kind as string) || 'login')
const rows = ref<any[]>([])
const loading = ref(false)
const more = ref(true)
const error = ref('')
const PAGE = 100

async function load(reset = true) {
  loading.value = true; error.value = ''
  try {
    const before = reset ? 0 : rows.value[rows.value.length - 1]?.id || 0
    const res = await adminApi<any[]>('/audit', { query: { kind: kind.value, limit: PAGE, before } })
    rows.value = reset ? res : [...rows.value, ...res]
    more.value = res.length === PAGE
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(() => load())
watch(kind, () => load())


function details(e: any) {
  const d = e.details || {}
  if (e.action.startsWith('login.')) return d.ua ? String(d.ua).replace(/^Mozilla\/5\.0 \(([^)]*)\).*$/, '$1') : ''
  const parts: string[] = []
  if (d.destinations) parts.push('→ ' + d.destinations.join(', '))
  if (d.active === false) parts.push(t('admin.audit.detailDisabled'))
  if (d.admin === true) parts.push(t('admin.audit.detailAdmin'))
  if (d.mailboxes && Array.isArray(d.mailboxes)) parts.push(t('admin.audit.detailMailboxes', { n: d.mailboxes.length }))
  return parts.join(', ')
}
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.audit.loading')" />
    <AdminHeader :title="t('admin.audit.title')" :desc="t('admin.audit.desc')" />
    <div class="seg" role="tablist" style="align-self:flex-start">
      <button :class="{ on: kind === 'login' }" @click="kind = 'login'">{{ t('admin.audit.tabLogins') }}</button>
      <button :class="{ on: kind === 'admin' }" @click="kind = 'admin'">{{ t('admin.audit.tabActions') }}</button>
      <button :class="{ on: kind === 'sent' }" @click="kind = 'sent'">{{ t('admin.audit.tabSent') }}</button>
    </div>
    <div v-if="error" class="notice danger">{{ error }}</div>
    <section class="card">
      <div class="tbl-wrap">
        <table v-if="kind === 'sent'" class="tbl">
          <thead><tr><th>{{ t('admin.audit.when') }}</th><th>{{ t('admin.audit.who') }}</th><th>{{ t('admin.audit.fromAddress') }}</th><th>{{ t('admin.audit.to') }}</th><th>{{ t('admin.audit.subject') }}</th></tr></thead>
          <tbody>
            <tr v-for="r in rows" :key="r.id">
              <td class="num" style="text-align:left">{{ dateTime(r.at) }}</td>
              <td><span class="pill accent">{{ r.login }}</span></td>
              <td>{{ r.mailbox }}</td>
              <td style="max-width:260px;overflow-wrap:anywhere">{{ r.recipients.join(', ') }}</td>
              <td>{{ r.subject || t('common.noSubject') }}</td>
            </tr>
          </tbody>
        </table>
        <table v-else class="tbl">
          <thead><tr><th>{{ t('admin.audit.when') }}</th><th>{{ t('admin.audit.event') }}</th><th>{{ kind === 'login' ? t('common.login') : t('admin.audit.object') }}</th><th v-if="kind === 'admin'">{{ t('admin.audit.who') }}</th><th>IP</th><th>{{ t('admin.audit.details') }}</th></tr></thead>
          <tbody>
            <tr v-for="r in rows" :key="r.id">
              <td class="num" style="text-align:left">{{ dateTime(r.at) }}</td>
              <td><span :class="r.action === 'login.failed' ? 'pill bad' : r.action === 'login.ok' ? 'pill ok' : 'pill'">{{ actionLabel(r.action) }}</span></td>
              <td>{{ r.target }}</td>
              <td v-if="kind === 'admin'">{{ r.actor }}</td>
              <td class="num" style="text-align:left">{{ r.ip }}</td>
              <td class="muted" style="font-size:12.5px;max-width:280px;overflow-wrap:anywhere">{{ details(r) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!loading && !rows.length" class="empty-card">{{ t('admin.audit.empty') }}</div>
      </div>
      <div v-if="more && rows.length" class="more"><button class="btn" :disabled="loading" @click="load(false)">{{ loading ? t('common.loading') : t('admin.audit.showMore') }}</button></div>
    </section>
  </div>
</template>
