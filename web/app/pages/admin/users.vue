<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { adminApi, ago } from '~/composables/admin'
import { t } from '~/composables/i18n'

const users = ref<any[]>([])
const mailboxes = ref<any[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')
const editing = ref<any | null | undefined>(undefined) // undefined = closed, null = new

async function load() {
  loading.value = true; error.value = ''
  try {
    const [u, m] = await Promise.all([adminApi('/users'), adminApi('/mailboxes')])
    users.value = u; mailboxes.value = m
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

const shown = computed(() => {
  const s = q.value.trim().toLowerCase()
  if (!s) return users.value
  return users.value.filter(u => [u.login, u.name, ...u.mailboxes.map((m: any) => m.address)].join(' ').toLowerCase().includes(s))
})
function saved() { editing.value = undefined; load() }
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.users.loading')" />
    <AdminHeader :title="t('admin.users.title')" :desc="t('admin.users.desc')">
      <label class="search"><AppIcon name="search" /><span class="sr-only">{{ t('common.search') }}</span><input v-model="q" type="search" :placeholder="t('admin.users.searchPlaceholder')" /></label>
      <button class="btn primary" @click="editing = null"><AppIcon name="plus" />{{ t('admin.users.addUser') }}</button>
    </AdminHeader>

    <div v-if="error" class="notice danger">{{ error }}</div>
    <section class="card">
      <div class="tbl-wrap">
        <table class="tbl">
          <thead><tr><th>{{ t('admin.users.colUser') }}</th><th>{{ t('admin.users.role') }}</th><th>{{ t('admin.users.colMailboxes') }}</th><th>{{ t('admin.users.colActivity') }}</th><th>{{ t('admin.users.colStatus') }}</th></tr></thead>
          <tbody>
            <tr v-for="u in shown" :key="u.id" class="click" :class="{ off: !u.active }" tabindex="0" @click="editing = u" @keydown.enter="editing = u">
              <td><div class="cell-main">
                <AppAvatar :login="u.login" :name="u.name || u.login" style="width:32px;height:32px;font-size:12px" />
                <div><b>{{ u.name || u.login }}</b><span>{{ u.login }}</span></div>
              </div></td>
              <td><span :class="u.isAdmin ? 'pill accent' : 'pill'">{{ u.isAdmin ? t('admin.users.roleAdmin') : t('admin.users.roleUser') }}</span></td>
              <td><div class="pills">
                <span v-for="m in u.mailboxes" :key="m.mailboxId" class="pill" :title="m.canSend ? t('admin.users.readSend') : t('admin.users.readOnly')">
                  {{ m.address }}<template v-if="m.personal"> · {{ t('admin.users.personal') }}</template><template v-if="!m.canSend"> · {{ t('admin.users.readShort') }}</template>
                </span>
                <span v-if="!u.mailboxes.length" class="muted">{{ t('admin.users.noAccess') }}</span>
              </div></td>
              <td class="muted" style="white-space:nowrap">{{ ago(u.lastSeen) }}</td>
              <td><span :class="u.active ? 'pill ok' : 'pill bad'">{{ u.active ? t('admin.users.statusActive') : t('admin.users.statusOff') }}</span></td>
            </tr>
            <tr v-if="!loading && !shown.length"><td colspan="5" class="empty-card">{{ q ? t('admin.users.notFound') : t('admin.users.empty') }}</td></tr>
            <tr v-if="loading && !users.length"><td colspan="5" class="empty-card" style="height:160px"></td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <AdminUserEditor v-if="editing !== undefined" :user="editing" :mailboxes="mailboxes" @close="editing = undefined" @saved="saved" />
  </div>
</template>
