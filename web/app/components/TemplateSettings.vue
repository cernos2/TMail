<script setup lang="ts">
// Settings tab: reply templates. Everyone keeps personal templates; admins also edit shared ones.
import { ref, computed, onMounted } from 'vue'
import { mail } from '~/composables/mail'
import { tpl, loadTemplates, saveTemplate, deleteTemplate, type Template } from '~/composables/templates'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { aiName } from '~/composables/ai'
import { t } from '~/composables/i18n'

const edit = ref<Partial<Template> | null>(null)
const busy = ref(false)
const removing = ref(false)
const canEdit = (tp: Template) => tp.mine || (tp.shared && !!mail.me?.isAdmin)
const mailboxes = computed(() => [{ value: '', label: t('templates.anyMailbox') }, ...(mail.me?.accounts || []).map(a => ({ value: a.address, label: a.address }))])
onMounted(loadTemplates)

async function save() {
  const tp = edit.value!
  if (!tp.title?.trim() || !tp.body?.trim()) { toast(t('templates.required'), { kind: 'err' }); return }
  busy.value = true
  try { await saveTemplate(tp); edit.value = null; toast(t('templates.saved')) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
async function remove(tp: Template) {
  if (!await confirmDialog(t('templates.deleteConfirm', { title: tp.title }), '', { ok: t('common.delete'), danger: true })) return
  removing.value = true
  try { await deleteTemplate(tp.id) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { removing.value = false }
}
</script>

<template>
  <div class="tpl-set veil-host">
    <LoadingVeil :show="!tpl.loaded || busy || removing" :text="busy || removing ? t('common.saving') : t('templates.loading')" />
    <p class="hint" style="margin:0">{{ t('templates.intro', { ai: aiName() }) }}</p>
    <div v-if="!edit" class="tpl-list">
      <div v-for="tp in tpl.list" :key="tp.id" class="tpl-item">
        <div class="tpl-txt"><b>{{ tp.title }}</b><span class="hint">{{ tp.shared ? t('templates.shared') : t('templates.personal') }}{{ tp.mailbox ? ' · ' + tp.mailbox : '' }}</span><p>{{ tp.body }}</p></div>
        <span v-if="canEdit(tp)" class="tpl-btns">
          <button class="icon-btn" :title="t('common.edit')" :aria-label="t('common.edit')" @click="edit = { ...tp }"><AppIcon name="edit" /></button>
          <button class="icon-btn" :title="t('common.delete')" :aria-label="t('common.delete')" @click="remove(tp)"><AppIcon name="trash" /></button>
        </span>
      </div>
      <p v-if="tpl.loaded && !tpl.list.length" class="hint">{{ t('templates.empty') }}</p>
      <div><button class="btn" @click="edit = { title: '', body: '', mailbox: '', shared: false }"><AppIcon name="plus" />{{ t('templates.new') }}</button></div>
    </div>
    <form v-else class="form ev-form" @submit.prevent="save">
      <label class="f">{{ t('common.name') }}<input v-model="edit.title" type="text" :placeholder="t('templates.namePlaceholder')" /></label>
      <label class="f">{{ t('templates.text') }}<textarea v-model="edit.body" rows="8"></textarea></label>
      <label class="f">{{ t('templates.forMailbox') }}<AppSelect v-model="edit.mailbox as any" :options="mailboxes" :aria-label="t('templates.mailbox')" /></label>
      <label v-if="mail.me?.isAdmin && !edit.id" class="check"><input v-model="edit.shared" type="checkbox" />{{ t('templates.sharedCheck') }}</label>
      <div style="display:flex;gap:8px;justify-content:flex-end">
        <button type="button" class="btn" @click="edit = null">{{ t('common.cancel') }}</button>
        <button class="btn primary" :disabled="busy">{{ busy ? t('common.saving') : t('common.save') }}</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.tpl-set{display:flex;flex-direction:column;gap:14px}
.tpl-list{display:flex;flex-direction:column;gap:8px}
.tpl-item{display:flex;gap:10px;border:1px solid var(--line);border-radius:5px;padding:10px 12px}
.tpl-txt{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px}
.tpl-txt p{margin:4px 0 0;font-size:13px;color:var(--ink-2);white-space:pre-wrap;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden}
.tpl-btns{display:flex;gap:2px;align-items:flex-start}
</style>
