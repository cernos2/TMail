<script setup lang="ts">
// Sender black / white lists (company-wide). Applied by the spam filter when mail arrives.
import { ref, computed, onMounted } from 'vue'
import { adminApi, dateTime } from '~/composables/admin'
import { api } from '~/composables/api'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

type Rule = { id: number, kind: 'block' | 'allow', pattern: string, name: string, note: string, createdBy: string, createdAt: string }
const list = ref<Rule[]>([])
const tab = ref<'block' | 'allow'>('block')
const q = ref('')
const add = ref('')
const note = ref('')
const picked = ref(new Set<number>())

const loaded = ref(false)
const working = ref('')
async function load() { try { list.value = await adminApi<Rule[]>('/sender-rules') } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { loaded.value = true } }
onMounted(load)
const shown = computed(() => list.value.filter(r => r.kind === tab.value && (!q.value || `${r.pattern} ${r.name} ${r.note}`.toLowerCase().includes(q.value.toLowerCase()))))
const count = (k: string) => list.value.filter(r => r.kind === k).length

async function submit() {
  if (!add.value.trim()) return
  working.value = t('admin.senders.adding')
  try {
    const r = await api<{ added: number, invalid: string[] }>('/api/sender-rules', { method: 'POST', body: { kind: tab.value, entries: [add.value], note: note.value } })
    toast(r.invalid.length ? t('admin.senders.addedInvalid', { n: r.added, invalid: r.invalid.join(', ') }) : t('admin.senders.added', { n: r.added }), { kind: r.invalid.length ? 'err' : 'info', ms: 6000 })
    add.value = ''; note.value = ''
    await load()
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { working.value = '' }
}
async function remove(ids: number[]) {
  if (ids.length > 1 && !await confirmDialog(t('admin.senders.removeConfirm', { n: ids.length }), '', { ok: t('admin.senders.removeOk') })) return
  working.value = t('admin.senders.removing')
  try {
    for (const id of ids) await adminApi(`/sender-rules/${id}`, { method: 'DELETE' })
    picked.value = new Set()
    await load()
  } finally { working.value = '' }
}
async function moveTo(r: Rule, kind: 'block' | 'allow') {
  working.value = t('admin.senders.moving')
  try {
    await api('/api/sender-rules', { method: 'POST', body: { kind, entries: [r.name ? `${r.name} <${r.pattern}>` : r.pattern], note: r.note } })
    toast(kind === 'allow' ? t('admin.senders.movedAllow') : t('admin.senders.movedBlock')); await load()
  } finally { working.value = '' }
}
function togglePick(id: number) { const s = new Set(picked.value); s.has(id) ? s.delete(id) : s.add(id); picked.value = s }
const allPicked = computed(() => shown.value.length > 0 && shown.value.every(r => picked.value.has(r.id)))
function pickAll() { picked.value = allPicked.value ? new Set() : new Set(shown.value.map(r => r.id)) }
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="!loaded || !!working" :text="working || t('admin.senders.loading')" />
    <AdminHeader :title="t('admin.senders.title')" :desc="t('admin.senders.desc')" />

    <div class="tabs sl-tabs" role="tablist">
      <button :class="{ on: tab === 'block' }" @click="tab = 'block'; picked = new Set()"><AppIcon name="shieldX" />{{ t('admin.senders.tabBlocked', { n: count('block') }) }}</button>
      <button :class="{ on: tab === 'allow' }" @click="tab = 'allow'; picked = new Set()"><AppIcon name="shield" />{{ t('admin.senders.tabAllowed', { n: count('allow') }) }}</button>
    </div>

    <section class="card">
      <form class="card-body sl-add" @submit.prevent="submit">
        <label class="f">{{ tab === 'block' ? t('admin.senders.addLabelBlock') : t('admin.senders.addLabelAllow') }}
          <textarea v-model="add" rows="3" :placeholder="t('admin.senders.addPlaceholder')"></textarea></label>
        <div class="sl-row">
          <input v-model="note" type="text" :placeholder="t('admin.senders.notePlaceholder')" />
          <button class="btn primary" :disabled="!add.trim()"><AppIcon name="plus" />{{ t('common.add') }}</button>
        </div>
        <p class="hint" style="margin:0">{{ tab === 'block' ? t('admin.senders.hintBlock') : t('admin.senders.hintAllow') }}</p>
      </form>
    </section>

    <section class="card">
      <div class="card-head sl-head">
        <input type="checkbox" :checked="allPicked" :indeterminate.prop="picked.size > 0 && !allPicked" :aria-label="t('admin.senders.selectAll')" @change="pickAll" />
        <button v-if="picked.size" class="btn sm" @click="remove([...picked])"><AppIcon name="trash" />{{ tab === 'block' ? t('admin.senders.bulkUnblock', { n: picked.size }) : t('admin.senders.bulkRemove', { n: picked.size }) }}</button>
        <span class="grow"></span>
        <label class="search"><AppIcon name="search" /><input v-model="q" type="search" :placeholder="t('common.search')" /></label>
      </div>
      <div v-for="r in shown" :key="r.id" class="sl-item">
        <input type="checkbox" :checked="picked.has(r.id)" :aria-label="r.pattern" @change="togglePick(r.id)" />
        <div class="sl-main">
          <span><b v-if="r.name">{{ r.name }}</b> <span :class="{ dim: !!r.name }">{{ r.name ? `<${r.pattern}>` : r.pattern }}</span>
            <span v-if="r.pattern.startsWith('@')" class="badge">{{ t('admin.senders.wholeDomain') }}</span></span>
          <span class="hint">{{ r.note ? r.note + ' · ' : '' }}{{ r.createdBy }}, {{ dateTime(r.createdAt) }}</span>
        </div>
        <button class="link" @click="moveTo(r, r.kind === 'block' ? 'allow' : 'block')">{{ r.kind === 'block' ? t('admin.senders.toAllowed') : t('admin.senders.block') }}</button>
        <button class="link" @click="remove([r.id])">{{ r.kind === 'block' ? t('admin.senders.unblock') : t('admin.senders.remove') }}</button>
      </div>
      <p v-if="!shown.length" class="card-body hint">{{ q ? t('admin.senders.notFound') : tab === 'block' ? t('admin.senders.emptyBlocked') : t('admin.senders.emptyAllowed') }}</p>
    </section>
  </div>
</template>

<style scoped>
.sl-tabs{margin-bottom:-6px}
.sl-tabs button{display:inline-flex;align-items:center;gap:6px}
.sl-tabs svg{width:16px;height:16px}
.sl-add{display:flex;flex-direction:column;gap:10px}
.sl-add textarea{min-height:80px;border:1px solid var(--line);border-radius:4.5px;padding:8px 10px;background:var(--surface-2);color:var(--ink);font:inherit;font-size:14px;resize:vertical;width:100%}
.sl-add .f{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--ink-2);font-weight:500}
.sl-row{display:flex;gap:8px;flex-wrap:wrap}
.sl-row input{flex:1;min-width:220px;height:36px;border:1px solid var(--line);border-radius:4.5px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit}
.sl-head{display:flex;align-items:center;gap:10px}
.sl-item{display:flex;align-items:center;gap:12px;padding:10px 16px;border-top:1px solid var(--line)}
.sl-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px;font-size:14px}
.sl-main .dim{color:var(--ink-2)}
.sl-main .badge{margin-left:6px}
</style>
