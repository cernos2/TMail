<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { adminApi, bytes } from '~/composables/admin'
import { t, fmtDate } from '~/composables/i18n'

const list = ref<any[]>([])
import { printSettings } from '~/composables/mailclient'
const domains = ref<any[]>([])
const users = ref<any[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')
const editing = ref<any | null | undefined>(undefined)

async function load() {
  loading.value = true; error.value = ''
  try {
    const [m, d, u] = await Promise.all([adminApi('/mailboxes'), adminApi('/domains'), adminApi('/users')])
    list.value = m; domains.value = d; users.value = u
    // /admin/mailboxes?open=<address> (from the mail collector) opens that mailbox's card.
    const open = useRoute().query.open
    if (typeof open === 'string' && editing.value === undefined) editing.value = list.value.find(x => x.address === open.toLowerCase()) ?? undefined
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

const shown = computed(() => {
  const s = q.value.trim().toLowerCase()
  return s ? list.value.filter(m => `${m.address} ${m.name} ${m.users.map((u: any) => u.login).join(' ')}`.toLowerCase().includes(s)) : list.value
})
const pct = (m: any) => m.quotaBytes && m.usedBytes != null ? Math.min(100, Math.round(m.usedBytes / m.quotaBytes * 100)) : 0
function saved() { editing.value = undefined; load() }
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.mailboxes.loading')" />
    <AdminHeader :title="t('admin.mailboxes.title')" :desc="t('admin.mailboxes.desc')">
      <label class="search"><AppIcon name="search" /><span class="sr-only">{{ t('common.search') }}</span><input v-model="q" type="search" :placeholder="t('admin.mailboxes.searchPlaceholder')" /></label>
      <button class="btn" :disabled="!list.length" :title="t('admin.mailboxes.printTitle')"
              @click="printSettings(list.filter(m => m.active && !m.external).map(m => ({ address: m.address, name: m.name || '' })))"><AppIcon name="download" />{{ t('admin.mailboxes.printSettings') }}</button>
      <button class="btn primary" :disabled="!domains.length" @click="editing = null"><AppIcon name="plus" />{{ t('admin.mailboxes.createMailbox') }}</button>
    </AdminHeader>

    <div v-if="error" class="notice danger">{{ error }}</div>
    <section class="card">
      <div class="tbl-wrap">
        <table class="tbl">
          <thead><tr><th>{{ t('admin.mailboxes.colMailbox') }}</th><th>{{ t('admin.mailboxes.colUsage') }}</th><th class="num">{{ t('admin.mailboxes.colLetters') }}</th><th>{{ t('admin.mailboxes.colUsers') }}</th><th>{{ t('admin.users.colStatus') }}</th></tr></thead>
          <tbody>
            <tr v-for="m in shown" :key="m.id" class="click" :class="{ off: !m.active }" tabindex="0" @click="editing = m" @keydown.enter="editing = m">
              <td><div class="cell-main"><div><b>{{ m.address }}<span v-if="m.external" class="badge" style="margin-left:6px" :title="t('admin.mailboxes.externalTitle')">{{ t('admin.mailboxes.external') }}</span></b><span>{{ m.name || '—' }}</span></div></div></td>
              <td>
                <div class="usage">
                  <div class="meter" :class="{ hot: pct(m) >= 80, full: pct(m) >= 95 }"><i :style="{ width: pct(m) + '%' }"></i></div>
                  <small>{{ t('admin.mailboxes.usedOf', { used: bytes(m.usedBytes), quota: m.quotaBytes ? bytes(m.quotaBytes) : '∞' }) }}</small>
                </div>
              </td>
              <td class="num">{{ m.messages ?? '—' }}</td>
              <td><div class="pills">
                <span v-for="u in m.users" :key="u.id" class="pill">{{ u.login }}{{ u.personal ? ' · ' + t('admin.users.personal') : '' }}</span>
                <span v-if="!m.users.length" class="muted">{{ t('admin.mailboxes.nobody') }}</span>
              </div></td>
              <td><span v-if="m.deletedAt" class="pill bad" :title="t('admin.mailboxes.purgeTitle', { date: fmtDate(m.purgeAfter, { dateStyle: 'short', timeStyle: 'medium' }) })">{{ t('admin.mailboxes.deletedPill', { date: fmtDate(m.purgeAfter, { day: 'numeric', month: 'short' }) }) }}</span>
                <span v-else :class="m.active ? 'pill ok' : 'pill bad'">{{ m.active ? t('admin.mailboxes.statusActive') : t('admin.mailboxes.statusOff') }}</span></td>
            </tr>
            <tr v-if="!loading && !shown.length"><td colspan="5" class="empty-card">{{ q ? t('admin.mailboxes.notFound') : t('admin.mailboxes.empty') }}</td></tr>
            <tr v-if="loading && !list.length"><td colspan="5" class="empty-card" style="height:160px"></td></tr>
          </tbody>
        </table>
      </div>
    </section>
    <p v-if="!domains.length && !loading" class="hint">{{ t('admin.mailboxes.addDomainFirst') }}</p>

    <AdminMailboxEditor v-if="editing !== undefined" :mailbox="editing" :domains="domains" :users="users" @close="editing = undefined" @saved="saved" />
  </div>
</template>
