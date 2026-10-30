<script setup lang="ts">
import { ref } from 'vue'
import { labels, saveLabel, deleteLabel, LABEL_COLORS } from '~/composables/labels'
import { mail, setView } from '~/composables/mail'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

const e = labels.edit!
const f = ref({ ...e })
const err = ref('')
const busy = ref('')
async function save() {
  if (!f.value.name.trim()) { err.value = t('label.errName'); return }
  busy.value = t('label.saving')
  try { await saveLabel(f.value); labels.edit = null; toast(e.id ? t('label.saved') : t('label.created', { name: f.value.name.trim() })) } catch (x: any) { err.value = x.message } finally { busy.value = '' }
}
async function remove() {
  if (!await confirmDialog(t('label.deleteConfirm', { name: e.name }), t('label.deleteHint'), { ok: t('common.delete'), danger: true })) return
  busy.value = t('label.deleting')
  try { await deleteLabel(e.id) } finally { busy.value = '' }
  if (mail.view.label === e.id) setView({ role: 'inbox' })
  labels.edit = null
}
</script>

<template>
  <AppModal :title="e.id ? t('label.title') : t('label.titleNew')" width="420px" :busy="busy" @close="labels.edit = null">
    <form class="form ev-form" @submit.prevent="save">
      <label class="f">{{ t('common.name') }}<input v-model="f.name" type="text" maxlength="40" :placeholder="t('label.namePlaceholder')" autofocus /></label>
      <div class="ev-colors" role="radiogroup" :aria-label="t('label.color')">
        <button v-for="(c, k) in LABEL_COLORS" :key="k" type="button" class="ev-color" :class="{ on: f.color === k }" :style="{ background: c.bg }"
                :title="c.label" :aria-label="c.label" role="radio" :aria-checked="f.color === k" @click="f.color = k as string"></button>
      </div>
      <p v-if="err" class="err">{{ err }}</p>
    </form>
    <template #foot>
      <button v-if="e.id" class="btn danger-text" type="button" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
      <span class="grow"></span>
      <button class="btn" type="button" @click="labels.edit = null">{{ t('common.cancel') }}</button>
      <button class="btn primary" type="button" @click="save">{{ e.id ? t('common.save') : t('common.create') }}</button>
    </template>
  </AppModal>
</template>
