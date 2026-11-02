<script setup lang="ts">
// Choice of a moment: ready options (tomorrow 9:00, next week…) and any date and time.
// Used for "Snooze", "Send later" and moving a scheduled letter.
import { ref, computed } from 'vue'
import { whenLabel, type WhenPreset } from '~/composables/later'
import { t } from '~/composables/i18n'

const props = defineProps<{ title: string, presets: WhenPreset[], ok?: string }>()
const emit = defineEmits<{ pick: [Date] }>()
const custom = ref(false)
const pad = (n: number) => String(n).padStart(2, '0')
const tomorrow = new Date(Date.now() + 864e5)
const date = ref(`${tomorrow.getFullYear()}-${pad(tomorrow.getMonth() + 1)}-${pad(tomorrow.getDate())}`)
const time = ref('09:00')
const today = (() => { const d = new Date(); return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` })()
const chosen = computed(() => { const d = new Date(`${date.value}T${time.value || '09:00'}:00`); return isNaN(d.getTime()) ? null : d })
const past = computed(() => !chosen.value || chosen.value.getTime() < Date.now() + 60e3)
function pickCustom() { if (chosen.value && !past.value) emit('pick', chosen.value) }
</script>

<template>
  <div class="when">
    <div class="when-title"><AppIcon name="clock" />{{ props.title }}</div>
    <template v-if="!custom">
      <button v-for="p in props.presets" :key="p.key" type="button" class="tp-item when-row" @click="emit('pick', p.at)">
        <span>{{ p.label }}</span><span class="when-at">{{ whenLabel(p.at) }}</span>
      </button>
      <div class="when-sep"></div>
      <button type="button" class="tp-item when-row" @click="custom = true"><span><AppIcon name="cal" />{{ t('later.pickDateTime') }}</span></button>
    </template>
    <div v-else class="when-custom">
      <label>{{ t('later.date') }}<DateInput v-model="date" :min="today" :aria-label="t('later.date')" /></label>
      <label>{{ t('later.time') }}<input v-model="time" type="time" /></label>
      <p v-if="chosen" class="hint" :class="{ err: past }">{{ past ? t('later.inPast') : whenLabel(chosen) }}</p>
      <div class="when-foot">
        <button type="button" class="btn sm ghost" @click="custom = false">{{ t('common.back') }}</button>
        <button type="button" class="btn sm primary" :disabled="past" @click="pickCustom">{{ props.ok || t('common.ok') }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.when{min-width:270px;display:flex;flex-direction:column}
.when-title{display:flex;align-items:center;gap:8px;font-weight:600;font-size:13px;padding:6px 10px 8px;color:var(--ink-2)}
.when-title svg{width:16px;height:16px;color:var(--accent)}
.when-row{display:flex;align-items:center;justify-content:space-between;gap:18px}
.when-row span:first-child{display:inline-flex;align-items:center;gap:8px}
.when-row svg{width:15px;height:15px;color:var(--ink-3)}
.when-at{color:var(--ink-3);font-size:12.5px;font-variant-numeric:tabular-nums}
.when-sep{height:1px;background:var(--line);margin:4px 6px}
.when-custom{display:flex;flex-direction:column;gap:10px;padding:4px 10px 8px}
.when-custom label{display:flex;flex-direction:column;gap:4px;font-size:12.5px;color:var(--ink-3)}
.when-custom input[type=time]{height:34px}
.when-custom .hint{margin:0;font-size:12.5px}
.when-custom .hint.err{color:var(--danger)}
.when-foot{display:flex;justify-content:flex-end;gap:8px}
</style>
