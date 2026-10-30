<script setup lang="ts">
import { translateServer } from '~/composables/servermsg'
import { ref, onMounted, watch } from 'vue'
import { adminApi } from '~/composables/admin'
import { toast } from '~/composables/toast'
import { confirmDialog, copyText } from '~/composables/dialogs'
import { t } from '~/composables/i18n'

const route = useRoute()
const domains = ref<any[]>([])
const sel = ref<number | null>(null)
const dns = ref<any | null>(null)
const checking = ref(false)
const error = ref('')
const adding = ref(false)
const newName = ref('')
const addErr = ref('')

const loaded = ref(false)
async function load() {
  try {
    domains.value = await adminApi('/domains')
    if (!sel.value) sel.value = Number(route.query.d) || domains.value[0]?.id || null
  } catch (e: any) { error.value = e.message } finally { loaded.value = true }
}
async function check() {
  if (!sel.value) return
  checking.value = true
  try { dns.value = await adminApi(`/domains/${sel.value}/dns`) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { checking.value = false }
}
onMounted(async () => { await load(); check() })
watch(sel, () => { dns.value = null; check() })

async function copy(text: string) { toast(await copyText(text) ? t('common.copied') : t('admin.domains.copyFailed')) }
async function addDomain() {
  addErr.value = ''
  try {
    const res = await adminApi('/domains', { method: 'POST', body: { name: newName.value.trim().toLowerCase() } })
    toast(t('admin.domains.added', { name: newName.value }))
    adding.value = false; newName.value = ''
    await load(); sel.value = res.id
  } catch (e: any) { addErr.value = e.message }
}
async function removeDomain(d: any) {
  if (!await confirmDialog(t('admin.domains.deleteConfirm', { name: d.name }), t('admin.domains.deleteConfirmText'), { ok: t('common.delete'), danger: true })) return
  try { await adminApi(`/domains/${d.id}`, { method: 'DELETE' }); toast(t('admin.domains.deleted')); sel.value = null; await load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}
const okCount = (checks: any[]) => checks.filter(c => c.status === 'ok').length
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="!loaded" :text="t('admin.domains.loading')" />
    <AdminHeader :title="t('admin.domains.title')" :desc="t('admin.domains.desc')">
      <button class="btn primary" @click="adding = true"><AppIcon name="plus" />{{ t('admin.domains.addDomain') }}</button>
    </AdminHeader>
    <div v-if="error" class="notice danger">{{ error }}</div>

    <div class="chips">
      <button v-for="d in domains" :key="d.id" class="chip" :class="{ on: sel === d.id }" style="height:32px;font-size:13px" @click="sel = d.id">
        <AppIcon name="globe" />{{ d.name }} <span class="muted">· {{ t('admin.domains.mailboxCount', { n: d.mailboxes }) }}</span>
      </button>
    </div>

    <section v-if="sel" class="card veil-host">
      <LoadingVeil :show="checking" :text="t('admin.domains.checkingDns')" />
      <div class="card-head">
        <h2>{{ t('admin.domains.dnsRecords', { domain: dns?.domain || '' }) }}</h2>
        <span v-if="dns" :class="okCount(dns.checks) === dns.checks.length ? 'pill ok' : 'pill bad'">{{ t('admin.domains.okOf', { ok: okCount(dns.checks), total: dns.checks.length }) }}</span>
        <button class="btn sm" :disabled="checking" @click="check"><AppIcon name="refresh" />{{ checking ? t('admin.domains.checking') : t('admin.domains.checkAgain') }}</button>
      </div>
      <div v-if="!dns" class="empty-card" style="min-height:200px"></div>
      <template v-else>
        <div v-if="okCount(dns.checks) < dns.checks.length" class="notice" style="margin:12px 16px 0">
          <AppIcon name="info" />
          <span>{{ t('admin.domains.whereToAdd') }}</span>
        </div>
        <div v-for="c in dns.checks" :key="c.key" class="dns-row">
          <span class="st" :class="c.status"><AppIcon :name="c.status === 'ok' ? 'check' : 'close'" /></span>
          <div style="min-width:0">
            <h3>{{ translateServer(c.title) }}
              <span :class="c.status === 'ok' ? 'pill ok' : 'pill bad'">{{ c.status === 'ok' ? t('admin.domains.statusOk') : c.status === 'missing' ? t('admin.domains.statusMissing') : t('admin.domains.statusWrong') }}</span></h3>
            <p v-if="c.hint" class="hint">{{ translateServer(c.hint) }}</p>
            <dl class="rec">
              <dt>{{ t('admin.domains.recType') }}</dt><dd><code>{{ c.type }}</code></dd>
              <dt>{{ t('admin.domains.recName') }}</dt><dd><code>{{ c.name }}</code><button v-if="c.name !== '—'" class="copy" @click="copy(c.name)">{{ t('common.copy') }}</button></dd>
              <template v-if="c.status !== 'ok'">
                <dt>{{ t('admin.domains.recExpected') }}</dt><dd><code>{{ c.expected }}</code><button v-if="c.expected && c.expected !== '—'" class="copy" @click="copy(c.expected)">{{ t('common.copy') }}</button></dd>
              </template>
              <template v-if="c.found">
                <dt>{{ t('admin.domains.recFound') }}</dt><dd><code>{{ c.found }}</code></dd>
              </template>
            </dl>
          </div>
        </div>
      </template>
      <div class="drawer-foot" v-if="domains.find(d => d.id === sel)">
        <span class="hint">{{ t('admin.domains.dkimSelector', { selector: domains.find(d => d.id === sel)?.dkimSelector || '—' }) }}</span>
        <span class="grow"></span>
        <button class="btn danger sm" @click="removeDomain(domains.find(d => d.id === sel))"><AppIcon name="trash" />{{ t('admin.domains.deleteDomain') }}</button>
      </div>
    </section>

    <AppModal v-if="adding" :title="t('admin.domains.newDomain')" width="480px" @close="adding = false">
      <div class="form">
        <label class="f">{{ t('admin.domains.domainName') }}<input v-model="newName" type="text" placeholder="example.com" autocapitalize="none" spellcheck="false" @keydown.enter="addDomain" /></label>
        <p class="hint" style="margin:0">{{ t('admin.domains.addHint') }}</p>
        <p v-if="addErr" class="err">{{ addErr }}</p>
      </div>
      <template #foot>
        <button class="btn" @click="adding = false">{{ t('common.cancel') }}</button>
        <button class="btn primary" @click="addDomain">{{ t('common.add') }}</button>
      </template>
    </AppModal>
  </div>
</template>
