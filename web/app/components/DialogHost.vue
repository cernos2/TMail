<script setup lang="ts">
import { ref } from 'vue'
import { confirmState, closeConfirm, secretState, closeSecret, copyText } from '~/composables/dialogs'
import { t } from '~/composables/i18n'
const copied = ref(false)
async function copy() { copied.value = await copyText(secretState.info?.secret || ''); setTimeout(() => copied.value = false, 2000) }
</script>

<template>
  <AppModal v-if="confirmState.open" :title="confirmState.title" width="460px" top @close="closeConfirm(false)">
    <p v-if="confirmState.text" style="margin:0;color:var(--ink-2);white-space:pre-line">{{ confirmState.text }}</p>
    <template #foot>
      <button class="btn" @click="closeConfirm(false)">{{ t('common.cancel') }}</button>
      <button class="btn" :class="confirmState.danger ? 'danger solid' : 'primary'" @click="closeConfirm(true)">{{ confirmState.ok }}</button>
    </template>
  </AppModal>
  <AppModal v-if="secretState.open && secretState.info" :title="secretState.info.title" width="480px" @close="closeSecret">
    <div class="form">
      <div class="secret">{{ secretState.info.secret }}</div>
      <p style="margin:0;color:var(--ink-2);font-size:13.5px">{{ secretState.info.note || t('ui.savePassword') }}</p>
      <div v-if="secretState.info.rows?.length" class="card"><div class="kv">
        <template v-for="r in secretState.info.rows" :key="r[0]"><div>{{ r[0] }}</div><div>{{ r[1] }}</div></template>
      </div></div>
    </div>
    <template #foot>
      <button class="btn" @click="copy"><AppIcon name="check" v-if="copied" />{{ copied ? t('common.copied') : t('ui.copyPassword') }}</button>
      <button class="btn primary" @click="closeSecret">{{ t('common.done') }}</button>
    </template>
  </AppModal>
</template>
