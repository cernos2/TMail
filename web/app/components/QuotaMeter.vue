<script setup lang="ts">
// Space used in the mailbox (bottom of the folder column), like Gmail.
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { bytes } from '~/composables/admin'
import { t } from '~/composables/i18n'

type Q = { address: string, used: number, limit: number, messages: number }
const all = ref<Q[]>([])
async function load() { try { all.value = await api<Q[]>('/api/quota') } catch {} }
let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => { load(); timer = setInterval(load, 5 * 60 * 1000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })

const shown = computed(() => mail.view.account ? all.value.filter(q => q.address === mail.view.account) : all.value)
const pct = (q: Q) => q.limit ? Math.min(100, Math.round(q.used / q.limit * 100)) : 0
const level = (q: Q) => pct(q) >= 95 ? 'full' : pct(q) >= 80 ? 'hot' : ''
// Several mailboxes: one line (total), opened on click; a mailbox running out of space opens it.
const opened = ref(false)
try { opened.value = localStorage.getItem('tmail.quotaOpen') === '1' } catch {}
function toggle() { opened.value = !opened.value; try { localStorage.setItem('tmail.quotaOpen', opened.value ? '1' : '0') } catch {} }
const collapsible = computed(() => shown.value.length > 1)
const urgent = computed(() => shown.value.some(q => level(q) !== ''))
const open = computed(() => !collapsible.value || opened.value || urgent.value)
const totalUsed = computed(() => shown.value.reduce((n, q) => n + q.used, 0))
</script>

<template>
  <div v-if="shown.length" class="quota" :aria-label="t('side.quotaAll')">
    <button v-if="collapsible" type="button" class="quota-head" :aria-expanded="open" @click="toggle">
      <span>{{ t('side.quotaAllN', { n: shown.length }) }}</span><span class="quota-sum">{{ bytes(totalUsed) }}</span>
      <svg class="quota-caret" :class="{ open }" viewBox="0 0 10 10" aria-hidden="true"><path d="M2 6l3-3 3 3" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>
    </button>
    <div v-for="q in (open ? shown : [])" :key="q.address" class="quota-item" :title="t('side.quotaLetters', { address: q.address, n: q.messages })">
      <div class="quota-top">
        <span v-if="shown.length > 1" class="quota-addr">{{ q.address }}</span>
        <span v-else class="quota-addr">{{ t('side.quotaOne') }}</span>
        <span class="quota-pct" :class="level(q)">{{ q.limit ? pct(q) + '%' : '' }}</span>
      </div>
      <div class="meter" :class="level(q)"><i :style="{ width: Math.max(pct(q), q.used ? 1 : 0) + '%' }"></i></div>
      <span class="quota-txt">{{ q.limit ? t('side.quotaUsedOf', { used: bytes(q.used), limit: bytes(q.limit) }) : t('side.quotaUsed', { used: bytes(q.used) }) }}</span>
      <span v-if="level(q) === 'full'" class="quota-warn">{{ t('side.quotaFull') }}</span>
    </div>
  </div>
</template>

<style scoped>
.quota{padding:10px 14px 12px;border-top:1px solid var(--line);display:flex;flex-direction:column;gap:10px;flex:none}
.quota-item{display:flex;flex-direction:column;gap:4px}
.quota-head{display:flex;align-items:center;gap:8px;border:0;background:none;padding:0;color:var(--ink-2);font:inherit;font-size:12px;cursor:pointer;text-align:left}
.quota-head:hover{color:var(--accent)}
.quota-head span:first-child{flex:1}
.quota-sum{font-family:var(--f-mono);font-size:11.5px;color:var(--ink-3)}
.quota-caret{width:10px;height:10px;transform:rotate(180deg);transition:transform .15s}
.quota-caret.open{transform:none}
.quota-top{display:flex;justify-content:space-between;gap:8px;font-size:12px;color:var(--ink-2)}
.quota-addr{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.quota-pct{font-family:var(--f-mono);font-size:11.5px;color:var(--ink-3)}
.quota-pct.hot{color:#e8a317}.quota-pct.full{color:var(--danger)}
.quota-txt{font-size:11.5px;color:var(--ink-3)}
.quota-warn{font-size:11.5px;color:var(--danger)}
.meter{height:5px}
</style>
