<script setup lang="ts">
// The user's interface language and time zone. "Automatic" keeps the server's language and the time
// zone of the device the mail is opened on.
import { ref, computed } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { LANGS, applyUserLocale, browserTz, t } from '~/composables/i18n'

const lang = ref(mail.me?.lang || '')
const tz = ref(mail.me?.timezone || '')
const busy = ref(false)

const serverLang = computed(() => LANGS.find(l => l.code === mail.me?.defaults?.lang)?.name || mail.me?.defaults?.lang || 'English')
const langOptions = computed(() => [{ value: '', label: t('locale.langAuto', { lang: serverLang.value }) },
  ...LANGS.map(l => ({ value: l.code, label: l.name === l.en ? l.name : `${l.name} — ${l.en}` }))])
const zones: string[] = (() => { try { return (Intl as any).supportedValuesOf('timeZone') } catch { return ['UTC'] } })()
const tzOptions = computed(() => [{ value: '', label: t('locale.tzAuto', { tz: browserTz() }) }, ...zones.map(z => ({ value: z, label: z.replace(/_/g, ' ') }))])

async function save() {
  busy.value = true
  try {
    await api('/api/me/locale', { method: 'PUT', body: { lang: lang.value, timezone: tz.value } })
    if (mail.me) { mail.me.lang = lang.value; mail.me.timezone = tz.value }
    await applyUserLocale(mail.me)
    toast(t('locale.saved'))
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
</script>

<template>
  <div class="locale-set">
    <label class="f">{{ t('locale.language') }}
      <AppSelect v-model="lang" :options="langOptions" :aria-label="t('locale.language')" searchable /></label>
    <label class="f">{{ t('locale.timezone') }}
      <AppSelect v-model="tz" :options="tzOptions" :aria-label="t('locale.timezone')" searchable /></label>
    <div><button class="btn primary" :disabled="busy || (lang === (mail.me?.lang || '') && tz === (mail.me?.timezone || ''))" @click="save">{{ busy ? t('common.saving') : t('common.save') }}</button></div>
  </div>
</template>

<style scoped>
.locale-set{display:flex;flex-direction:column;gap:12px;max-width:440px}
</style>
