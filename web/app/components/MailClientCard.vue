<script setup lang="ts">
// Settings card for one mailbox in mail programs, with copy buttons, short how-tos,
// print / text / iPhone profile. The password appears only right after it was set.
import { computed } from 'vue'
import { clientSettings, settingsText, mobileconfig, printSettings } from '~/composables/mailclient'
import { copyText } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

const props = defineProps<{ address: string, name?: string, password?: string }>()
const rows = computed(() => clientSettings(props.address, props.password))
async function copy(v: string, what = t('common.copied')) { if (await copyText(v)) toast(what) }
</script>

<template>
  <div class="mcc">
    <div class="mcc-head">
      <b>{{ props.address }}</b><span v-if="props.name" class="hint">{{ props.name }}</span>
      <span class="grow"></span>
      <button type="button" class="btn sm" :title="t('mailclient.copyAllTitle')" @click="copy(settingsText(props.address, props.name || '', props.password), t('mailclient.settingsCopied'))"><AppIcon name="copy" />{{ t('mailclient.asText') }}</button>
      <button type="button" class="btn sm" :title="t('mailclient.printTitle')" @click="printSettings([{ address: props.address, name: props.name || '', password: props.password }])"><AppIcon name="download" />{{ t('mailclient.print') }}</button>
      <button type="button" class="btn sm" :title="t('mailclient.appleTitle')" @click="mobileconfig(props.address, props.name || '', props.password)"><AppIcon name="settings" />{{ t('mailclient.apple') }}</button>
    </div>
    <dl class="mcc-rows">
      <template v-for="r in rows" :key="r.key">
        <dt>{{ r.label }}</dt>
        <dd :class="{ secret: r.key === 'pass' && props.password, muted: r.key === 'pass' && !props.password }">
          <span>{{ r.value }}<small v-if="r.alt">{{ r.alt }}</small></span>
          <button v-if="r.copy" type="button" class="icon-btn" :aria-label="t('mailclient.copyField', { label: r.label })" @click="copy(r.value)"><AppIcon name="copy" /></button>
        </dd>
      </template>
    </dl>
    <p v-if="props.password" class="mcc-warn">{{ t('mailclient.passwordOnce') }}</p>
    <details class="mcc-how">
      <summary>{{ t('mailclient.howTo') }}</summary>
      <ul>
        <li><b>iPhone / iPad / Mac</b> {{ t('mailclient.howApple') }}</li>
        <li><b>Android</b> {{ t('mailclient.howAndroid') }}</li>
        <li><b>Outlook</b> {{ t('mailclient.howOutlook') }}</li>
        <li><b>Thunderbird</b> {{ t('mailclient.howThunderbird') }}</li>
      </ul>
    </details>
  </div>
</template>

<style scoped>
.mcc{border:1px solid var(--line);border-radius:6px;padding:12px 14px;background:var(--surface);display:flex;flex-direction:column;gap:10px}
.mcc-head{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.mcc-head b{font-size:15px}
.mcc-rows{display:grid;grid-template-columns:minmax(150px,38%) 1fr;gap:6px 14px;margin:0;font-size:14px}
.mcc-rows dt{color:var(--ink-3)}
.mcc-rows dd{margin:0;display:flex;align-items:flex-start;gap:4px;font-weight:600;user-select:text}
.mcc-rows dd > span{display:flex;flex-direction:column}
.mcc-rows small{font-weight:400;color:var(--ink-3);font-size:12.5px}
.mcc-rows dd.secret span{font-family:var(--f-mono);background:var(--accent-soft);padding:1px 8px;border-radius:3px}
.mcc-rows dd.muted{font-weight:400;color:var(--ink-2)}
.mcc-rows .icon-btn{width:24px;height:24px}
.mcc-rows .icon-btn svg{width:14px;height:14px}
.mcc-warn{margin:0;font-size:13px;color:#b7791f}
.mcc-how summary{cursor:pointer;color:var(--accent);font-size:13.5px}
.mcc-how ul{margin:8px 0 0;padding-left:18px;display:flex;flex-direction:column;gap:6px;font-size:13.5px;color:var(--ink-2)}
@media (max-width:760px){.mcc-rows{grid-template-columns:1fr}.mcc-rows dt{margin-top:4px}}
</style>
