<script setup lang="ts">
// Signatures and start templates of the mailboxes the user works with.
// Personal mailboxes are editable by their owner; shared ones by administrators.
import { ref, reactive, computed } from 'vue'
import { mail, sigText } from '~/composables/mail'
import { api } from '~/composables/api'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

const drafts = reactive<Record<string, { signature: string, template: string, saving: boolean }>>({})
for (const a of mail.me?.accounts || []) drafts[a.address] = { signature: a.signature, template: a.template, saving: false }

// The address new letters are written from (each user has their own choice, kept on the server).
const fromOptions = computed(() => [{ value: '', label: t('signature.currentMailbox') },
  ...(mail.me?.accounts || []).filter(a => a.canSend).map(a => ({ value: a.address, label: a.address }))])
const savingFrom = ref(false)
async function setDefaultFrom(v: string | number) {
  mail.defaultFrom = String(v)
  savingFrom.value = true
  try { await api('/api/settings', { method: 'PUT', body: { defaultFrom: mail.defaultFrom } }); toast(t('common.saved')) } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { savingFrom.value = false }
}

async function save(addr: string) {
  const d = drafts[addr]
  d.saving = true
  try {
    await api('/api/mailbox-settings', { method: 'PUT', body: { address: addr, signature: d.signature, template: d.template } })
    const acc = mail.me?.accounts.find(a => a.address === addr)
    if (acc) { acc.signature = d.signature.trimEnd(); acc.template = d.template }
    toast(t('signature.saved'))
  } catch (e: any) {
    toast(e.message, { kind: 'err' })
  } finally { d.saving = false }
}
</script>

<template>
  <div class="veil-host">
    <LoadingVeil :show="savingFrom" :text="t('common.saving')" />
    <div class="f def-from">{{ t('signature.defaultFrom') }}
      <AppSelect :model-value="mail.defaultFrom" :options="fromOptions" :aria-label="t('signature.defaultFrom')" @update:model-value="setDefaultFrom" />
      <span class="hint">{{ t('signature.defaultFromHint') }}</span>
    </div>
    <p class="hint" style="margin:0 0 4px">{{ t('signature.placeholdersHint', { name: '{name}', login: '{login}', email: '{email}' }) }}</p>
    <div v-for="a in mail.me?.accounts || []" :key="a.address" class="sig-edit form veil-host">
      <LoadingVeil class="compact" :show="!!drafts[a.address]?.saving" :text="t('signature.saving')" />
      <h3>{{ a.address }}
        <span v-if="a.personal" class="pill accent">{{ t('signature.personalMailbox') }}</span>
        <span v-else class="pill">{{ t('signature.sharedMailbox') }}</span>
      </h3>
      <template v-if="a.canEditSignature">
        <div class="f">{{ t('signature.signature') }} <span class="help">{{ t('signature.signatureHelp') }}</span>
          <SignatureEditor v-model="drafts[a.address].signature" :placeholder="t('signature.signaturePlaceholder', { name: '{name}' })" />
        </div>
        <label class="f">{{ t('signature.startTemplate') }} <span class="help">{{ t('signature.startTemplateHelp') }}</span>
          <textarea v-model="drafts[a.address].template" rows="3" :placeholder="t('signature.startTemplatePlaceholder')" />
        </label>
        <div><button class="btn primary sm" :disabled="drafts[a.address].saving" @click="save(a.address)">{{ drafts[a.address].saving ? t('common.saving') : t('common.save') }}</button></div>
      </template>
      <p v-else class="hint" style="margin:0">{{ t('signature.adminOnly') }}<template v-if="a.signature"> {{ t('signature.current') }} <span style="white-space:pre-line">{{ sigText(a.signature) }}</span></template></p>
    </div>
  </div>
</template>

<style scoped>
.def-from{display:flex;flex-direction:column;gap:6px;margin-bottom:16px;padding-bottom:14px;border-bottom:1px solid var(--line);font-size:13px;font-weight:500;color:var(--ink-2)}
</style>
