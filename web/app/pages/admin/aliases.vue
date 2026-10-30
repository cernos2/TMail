<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { adminApi } from '~/composables/admin'
import { toast } from '~/composables/toast'
import { confirmDialog } from '~/composables/dialogs'
import { t } from '~/composables/i18n'

const list = ref<any[]>([])
const domains = ref<any[]>([])
const mailboxes = ref<any[]>([])
const loading = ref(true)
const error = ref('')
const q = ref('')

// editor
const editing = ref<any | null | undefined>(undefined)
const form = reactive({ local: '', domain: '', dests: '', active: true })
const formErr = ref('')
const saving = ref(false)

async function load() {
  loading.value = true; error.value = ''
  try {
    const [a, d, m] = await Promise.all([adminApi('/aliases'), adminApi('/domains'), adminApi('/mailboxes')])
    list.value = a; domains.value = d; mailboxes.value = m
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
onMounted(load)

const shown = computed(() => {
  const s = q.value.trim().toLowerCase()
  return s ? list.value.filter(a => `${a.source} ${a.destinations.join(' ')}`.includes(s)) : list.value
})

function open(a: any | null) {
  editing.value = a; formErr.value = ''
  form.local = ''; form.domain = domains.value[0]?.name || ''
  form.dests = a ? a.destinations.join('\n') : ''
  form.active = a ? a.active : true
}
function addDest(addr: string) {
  const cur = form.dests.split(/[\s,;]+/).filter(Boolean)
  if (!cur.includes(addr)) form.dests = [...cur, addr].join('\n')
}
async function save() {
  formErr.value = ''
  const destinations = form.dests.split(/[\s,;]+/).map(s => s.trim().toLowerCase()).filter(Boolean)
  if (!destinations.length) { formErr.value = t('admin.aliases.needDestination'); return }
  saving.value = true
  try {
    if (editing.value) {
      await adminApi(`/aliases/${editing.value.id}`, { method: 'PATCH', body: { destinations, active: form.active } })
      toast(t('admin.aliases.saved'))
    } else {
      const source = `${form.local.trim().toLowerCase()}@${form.domain}`
      await adminApi('/aliases', { method: 'POST', body: { source, destinations } })
      toast(t('admin.aliases.created', { source }))
    }
    editing.value = undefined
    load()
  } catch (e: any) { formErr.value = e.message } finally { saving.value = false }
}
async function remove(a: any) {
  if (!await confirmDialog(t('admin.aliases.deleteConfirm', { source: a.source }), t('admin.aliases.deleteConfirmText'), { ok: t('common.delete'), danger: true })) return
  try { await adminApi(`/aliases/${a.id}`, { method: 'DELETE' }); toast(t('admin.aliases.deleted')); editing.value = undefined; load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading" :text="t('admin.aliases.loading')" />
    <AdminHeader :title="t('admin.aliases.title')" :desc="t('admin.aliases.desc')">
      <label class="search"><AppIcon name="search" /><span class="sr-only">{{ t('common.search') }}</span><input v-model="q" type="search" :placeholder="t('admin.aliases.address')" /></label>
      <button class="btn primary" :disabled="!domains.length" @click="open(null)"><AppIcon name="plus" />{{ t('admin.aliases.add') }}</button>
    </AdminHeader>
    <div v-if="error" class="notice danger">{{ error }}</div>
    <section class="card">
      <div class="tbl-wrap">
        <table class="tbl">
          <thead><tr><th>{{ t('admin.aliases.address') }}</th><th>{{ t('admin.aliases.forwardsTo') }}</th><th>{{ t('admin.aliases.status') }}</th></tr></thead>
          <tbody>
            <tr v-for="a in shown" :key="a.id" class="click" :class="{ off: !a.active }" tabindex="0" @click="open(a)" @keydown.enter="open(a)">
              <td><b style="font-weight:600">{{ a.source }}</b></td>
              <td><div class="pills"><span v-for="d in a.destinations" :key="d" class="pill">{{ d }}</span></div></td>
              <td><span :class="a.active ? 'pill ok' : 'pill bad'">{{ a.active ? t('admin.aliases.active') : t('admin.aliases.inactive') }}</span></td>
            </tr>
            <tr v-if="!loading && !shown.length"><td colspan="3" class="empty-card">{{ q ? t('admin.aliases.notFound') : t('admin.aliases.empty') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <AppModal v-if="editing !== undefined" :title="editing ? editing.source : t('admin.aliases.newAlias')" width="560px" :busy="saving ? t('common.saving') : ''" @close="editing = undefined">
      <div class="form">
        <label v-if="!editing" class="f">{{ t('admin.aliases.aliasAddress') }}
          <div class="addon">
            <input v-model="form.local" type="text" placeholder="sales" autocapitalize="none" spellcheck="false" style="flex:1" />
            <span style="padding:0 4px">@</span>
            <AppSelect v-model="form.domain" :options="domains.map(d => ({ value: d.name, label: d.name }))" bare :aria-label="t('admin.aliases.domain')" />
          </div>
        </label>
        <label class="f">{{ t('admin.aliases.forwardTo') }}
          <textarea v-model="form.dests" rows="4" :placeholder="t('admin.aliases.onePerLine')" />
          <span class="help">{{ t('admin.aliases.externalHint') }}</span>
        </label>
        <div class="pills">
          <button v-for="m in mailboxes" :key="m.id" class="pill" type="button" style="cursor:pointer" @click="addDest(m.address)">+ {{ m.address }}</button>
        </div>
        <label v-if="editing" class="check"><input v-model="form.active" type="checkbox" />{{ t('admin.aliases.activeCheck') }}</label>
        <p v-if="formErr" class="err">{{ formErr }}</p>
      </div>
      <template #foot>
        <button v-if="editing" class="btn danger" @click="remove(editing)"><AppIcon name="trash" />{{ t('common.delete') }}</button>
        <span class="grow"></span>
        <button class="btn" @click="editing = undefined">{{ t('common.cancel') }}</button>
        <button class="btn primary" :disabled="saving" @click="save">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </template>
    </AppModal>
  </div>
</template>
