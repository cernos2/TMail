<script setup lang="ts">
// Create or edit a folder: name, mailbox, what it is for (the AI sorts by this), senders, auto-sorting.
import { ref, computed } from 'vue'
import { mail, setView } from '~/composables/mail'
import { folders, saveFolder, deleteFolder } from '~/composables/folders'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'
import { aiName } from '~/composables/ai'

const e = folders.edit!
const f = ref({ ...e })
// Delete letters N days after they arrived: a ready choice or any number of days.
const EXPIRE_OPTIONS = computed(() => [{ value: 0, label: t('folder.expireNever') }, { value: 7, label: t('folder.expireAfterDays', { n: 7 }) }, { value: 14, label: t('folder.expireAfterDays', { n: 14 }) },
  { value: 30, label: t('folder.expireAfterDays', { n: 30 }) }, { value: 90, label: t('folder.expire3Months') }, { value: 180, label: t('folder.expireHalfYear') }, { value: 365, label: t('folder.expireYear') }, { value: -1, label: t('folder.expireCustom') }])
f.value.expireDays = f.value.expireDays || 0
const expireMode = computed({
  get: () => EXPIRE_OPTIONS.value.some(o => o.value === f.value.expireDays && o.value !== -1) ? f.value.expireDays : -1,
  set: (v: number) => { f.value.expireDays = v === -1 ? (f.value.expireDays > 0 ? f.value.expireDays : 60) : v },
})
const senders = ref<string[]>(e.senders.split(/\s+/).map(x => x.trim()).filter(Boolean))
const busy = ref(false)
const err = ref('')
const accounts = computed(() => (mail.me?.accounts || []).map(a => ({ value: a.address, label: a.address })))

async function save() {
  if (!f.value.name.trim()) { err.value = t('folder.errName'); return }
  f.value.senders = senders.value.join('\n')
  if (!(f.value.expireDays >= 0 && f.value.expireDays <= 3650)) { err.value = t('folder.errExpire'); return }
  if (f.value.auto && !f.value.description.trim() && !f.value.senders.trim()) { err.value = t('folder.errAuto'); return }
  busy.value = true; err.value = ''
  try {
    await saveFolder(f.value)
    toast(e.create ? t('folder.created', { name: f.value.name.trim() }) : t('common.saved'))
    if (!e.create && e.folder !== f.value.name.trim() && mail.view.folder === e.folder) setView({ account: e.account, folder: f.value.name.trim() })
    folders.edit = null
  } catch (x: any) { err.value = x.message } finally { busy.value = false }
}
async function remove() {
  if (!await confirmDialog(t('folder.deleteConfirm', { name: e.folder }), t('folder.deleteOnlyEmpty'), { ok: t('common.delete'), danger: true })) return
  try {
    await deleteFolder(e.account, e.folder)
    if (mail.view.folder === e.folder) setView({ account: e.account, role: 'inbox' })
    folders.edit = null
    toast(t('folder.deleted'))
  } catch (x: any) { err.value = x.message }
}
</script>

<template>
  <AppModal :title="e.create ? t('folder.titleNew') : t('folder.title')" width="640px" :busy="busy ? t('folder.saving') : ''" @close="folders.edit = null">
    <form class="form ev-form" @submit.prevent="save">
      <label class="f">{{ t('common.name') }}<input v-model="f.name" type="text" maxlength="60" :placeholder="t('folder.namePlaceholder')" autofocus /></label>
      <label v-if="e.create && accounts.length > 1" class="f">{{ t('folder.mailbox') }}<AppSelect v-model="f.account" :options="accounts" :aria-label="t('folder.mailbox')" /></label>
      <label class="f">{{ t('folder.description') }}
        <textarea v-model="f.description" rows="4" :placeholder="t('folder.descriptionPlaceholder')"></textarea>
        <span class="help">{{ t('folder.descriptionHelp', { ai: aiName() }) }}</span></label>
      <div class="f">{{ t('folder.senders') }}
        <SendersEditor v-model="senders" />
        <span class="help">{{ t('folder.sendersHelp') }}</span></div>
      <label class="check"><input v-model="f.auto" type="checkbox" />{{ t('folder.auto') }}</label>
      <p v-if="f.auto" class="help" style="margin:-4px 0 0">{{ t('folder.autoHelp', { ai: aiName() }) }}</p>
      <label class="check"><input v-model="f.hideInbox" type="checkbox" @change="f.hideInbox && (f.auto = true)" />{{ t('folder.hideInbox') }}</label>
      <p v-if="f.hideInbox" class="help" style="margin:-4px 0 0">{{ t('folder.hideInboxHelp') }}</p>
      <div class="f">{{ t('folder.expire') }}
        <div class="exp-row">
          <AppSelect v-model="expireMode" :options="EXPIRE_OPTIONS" :aria-label="t('folder.expire')" />
          <template v-if="expireMode === -1"><input v-model.number="f.expireDays" type="number" min="1" max="3650" :aria-label="t('folder.expireDaysAria')" /> {{ t('folder.daysUnit') }}</template>

        </div>
        <span v-if="f.expireDays > 0" class="help">{{ t('folder.expireHelp', { n: f.expireDays }) }}</span></div>
      <p v-if="err" class="err" role="alert">{{ err }}</p>
    </form>
    <template #foot>
      <button v-if="!e.create" class="btn danger-text" type="button" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
      <span class="grow"></span>
      <button class="btn" type="button" @click="folders.edit = null">{{ t('common.cancel') }}</button>
      <button class="btn primary" type="button" :disabled="busy" @click="save">{{ busy ? t('common.saving') : e.create ? t('common.create') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>

<style scoped>
.exp-row{display:flex;align-items:center;gap:8px;font-weight:400}
.exp-row input{width:90px;height:34px;border:1px solid var(--line);border-radius:4px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit}
</style>
