<script setup lang="ts">
// Auto-reply per mailbox: reason, dates, who replaces you; the AI assistant writes the text, you can edit it.
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { hasAI, aiName } from '~/composables/ai'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

type AR = { mailbox: string, enabled: boolean, reason: string, dateFrom: string, dateTo: string, substitute: string, notes: string, subject: string, body: string, updatedBy?: string }
const reasons = computed(() => [
  { value: 'vacation', label: t('autoreply.reasonVacation') }, { value: 'trip', label: t('autoreply.reasonTrip') }, { value: 'sick', label: t('autoreply.reasonSick') },
  { value: 'holiday', label: t('autoreply.reasonHoliday') }, { value: 'dayoff', label: t('autoreply.reasonDayoff') }, { value: 'other', label: t('autoreply.reasonOther') },
])
const boxes = computed(() => (mail.me?.accounts || []).filter(a => a.canSend).map(a => ({ value: a.address, label: a.address })))
const box = ref(boxes.value.find(b => mail.me?.accounts.find(a => a.address === b.value)?.personal)?.value || boxes.value[0]?.value || '')
const f = ref<AR | null>(null)
const busy = ref('')
const today = new Date().toISOString().slice(0, 10)

async function load() {
  if (!box.value) return
  f.value = null
  try { f.value = await api<AR>('/api/autoreply', { query: { mailbox: box.value } }) } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
watch(box, load)
onMounted(load)

async function draft() {
  if (!f.value) return
  busy.value = 'draft'
  try {
    const r = await api<{ subject: string, body: string }>('/api/autoreply/draft', { method: 'POST', body: f.value, timeout: 160000 })
    f.value.body = r.body.replace(/\*\*(.+?)\*\*/g, '$1')
    f.value.subject = r.subject
  } catch (e: any) { toast(e.message, { kind: 'err', ms: 8000 }) } finally { busy.value = '' }
}
async function save(enable?: boolean) {
  if (!f.value) return
  if (enable !== undefined) f.value.enabled = enable
  busy.value = 'save'
  try {
    await api('/api/autoreply', { method: 'PUT', body: f.value })
    toast(f.value.enabled ? t('autoreply.enabledFor', { mailbox: f.value.mailbox }) : t('autoreply.off'))
  } catch (e: any) { toast(e.message, { kind: 'err', ms: 8000 }); if (enable) f.value.enabled = false } finally { busy.value = '' }
}
</script>

<template>
  <div class="ar veil-host">
    <LoadingVeil :show="!f || !!busy" :text="busy === 'draft' ? t('autoreply.aiWriting', { ai: aiName() }) : busy === 'save' ? t('common.saving') : t('autoreply.loading')" />
    <p class="hint" style="margin:0">{{ t('autoreply.intro') }}</p>
    <label v-if="boxes.length > 1" class="f">{{ t('autoreply.mailbox') }}<AppSelect v-model="box" :options="boxes" :aria-label="t('autoreply.mailbox')" /></label>
    <div v-if="!f" style="height:240px"></div>
    <form v-else class="form ev-form" @submit.prevent="save()">
      <div :class="['ar-state', { on: f.enabled }]">
        <b>{{ f.enabled ? t('autoreply.on') : t('autoreply.off') }}</b>
        <span class="grow"></span>
        <button type="button" :class="['btn', f.enabled ? '' : 'primary']" :disabled="!!busy" @click="save(!f.enabled)">{{ f.enabled ? t('autoreply.turnOff') : t('autoreply.turnOn') }}</button>
      </div>
      <div class="row2">
        <label class="f">{{ t('autoreply.reason') }}<AppSelect v-model="f.reason" :options="reasons" :aria-label="t('autoreply.reason')" /></label>
        <label class="f">{{ t('autoreply.substitute') }}<input v-model="f.substitute" type="text" :placeholder="t('autoreply.substitutePlaceholder')" /></label>
      </div>
      <div class="row2">
        <label class="f">{{ t('autoreply.from') }}<DateInput v-model="f.dateFrom" :min="today" :aria-label="t('autoreply.from')" /></label>
        <label class="f">{{ t('autoreply.toInclusive') }}<DateInput v-model="f.dateTo" :min="f.dateFrom || today" :aria-label="t('autoreply.to')" /></label>
      </div>
      <label class="f">{{ t('autoreply.notes') }}<input v-model="f.notes" type="text" :placeholder="t('autoreply.notesPlaceholder')" /></label>
      <div v-if="hasAI('reply')"><button type="button" class="btn ai" :disabled="!!busy" @click="draft"><AppIcon name="sparkle" />{{ busy === 'draft' ? t('autoreply.aiWritingShort', { ai: aiName() }) : f.body ? t('autoreply.rewrite', { ai: aiName() }) : t('autoreply.write', { ai: aiName() }) }}</button></div>
      <label class="f">{{ t('autoreply.subject') }}<input v-model="f.subject" type="text" :placeholder="t('autoreply.subjectPlaceholder')" /></label>
      <label class="f">{{ t('autoreply.body') }}<textarea v-model="f.body" rows="7" :placeholder="t('autoreply.bodyPlaceholder', { ai: aiName() })"></textarea></label>
      <div style="display:flex;justify-content:flex-end;gap:8px">
        <span v-if="f.updatedBy" class="hint" style="margin-right:auto;align-self:center">{{ t('autoreply.updatedBy', { name: f.updatedBy }) }}</span>
        <button class="btn primary" :disabled="!!busy">{{ busy === 'save' ? t('common.saving') : t('common.save') }}</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.ar{display:flex;flex-direction:column;gap:14px}
.ar-state{display:flex;align-items:center;gap:10px;border:1px solid var(--line);border-radius:6px;padding:10px 14px;background:var(--surface-2)}
.ar-state.on{border-color:var(--accent-line);background:var(--accent-soft)}
.ar input[type=date]{height:38px;border:1px solid var(--line);border-radius:4.5px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit;color-scheme:light dark}
</style>
