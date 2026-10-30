<script setup lang="ts">
// Senders of a folder rule: addresses grouped by domain (A→Z), a whole-domain entry shown on its
// group, search, add one or paste a list, remove one by one or a whole domain.
import { ref, computed } from 'vue'
import { t } from '~/composables/i18n'

const model = defineModel<string[]>({ required: true })
const q = ref('')
const add = ref('')
const err = ref('')

const norm = (s: string) => s.trim().toLowerCase().replace(/^[<"']+|[>"']+$/g, '')
function parse(text: string) {
  const out: string[] = []
  for (let p of text.split(/[\s,;]+/)) {
    p = norm(p)
    if (!p) continue
    if (!p.includes('@')) { if (!p.includes('.')) continue; p = '@' + p } // a bare domain
    if (!/^[^@\s]*@[^@\s]+\.[^@\s]+$/.test(p)) continue
    out.push(p)
  }
  return out
}
function addFrom(text: string) {
  const found = parse(text)
  if (!found.length) { err.value = t('senders.invalid'); return false }
  const have = new Set(model.value)
  model.value = [...model.value, ...found.filter(x => !have.has(x))]
  err.value = ''
  return true
}
function submit() { if (addFrom(add.value)) add.value = '' }
function onPaste(e: ClipboardEvent) {
  const text = e.clipboardData?.getData('text') || ''
  if (/[\s,;]/.test(text.trim())) { e.preventDefault(); addFrom(text) } // a list: add all at once
}
const remove = (x: string) => { model.value = model.value.filter(v => v !== x) }

// The company's domain: news.ozon.ru, seller.ozon.ru → ozon.ru (one group per company).
const baseOf = (d: string) => { const p = d.split('.'); return p.length > 2 && !/^(com|net|org|co|msk|spb)$/.test(p[p.length - 2]!) ? p.slice(-2).join('.') : d }
type Item = { value: string, label: string, whole: boolean }
const groups = computed(() => {
  const s = q.value.trim().toLowerCase()
  const by = new Map<string, { domain: string, whole: boolean, items: Item[] }>()
  for (const x of model.value) {
    if (s && !x.includes(s)) continue
    const [local, domain] = x.split('@') as [string, string]
    const base = baseOf(domain)
    const g = by.get(base) || { domain: base, whole: false, items: [] }
    const sub = domain === base ? '' : domain.slice(0, -base.length - 1)
    if (!local && !sub) g.whole = true
    else g.items.push({ value: x, label: local ? local + (sub ? '@' + sub : '') : t('senders.wholeDomainAt', { domain }), whole: !local })
    by.set(base, g)
  }
  for (const g of by.values()) g.items.sort((a, b) => Number(b.whole) - Number(a.whole) || a.label.localeCompare(b.label))
  return [...by.values()].sort((a, b) => a.domain.localeCompare(b.domain))
})
const removeDomain = (d: string) => { model.value = model.value.filter(v => baseOf(v.split('@')[1]!) !== d) }
</script>

<template>
  <div class="snd">
    <div class="snd-add">
      <input v-model="add" type="text" :placeholder="t('senders.addPlaceholder')" @keydown.enter.prevent="submit" @paste="onPaste" />
      <button type="button" class="btn sm" @click="submit"><AppIcon name="plus" />{{ t('common.add') }}</button>
    </div>
    <p v-if="err" class="err" style="margin:0">{{ err }}</p>
    <div v-if="model.length" class="snd-head">
      <span class="hint">{{ t('senders.count', { n: model.length, domains: new Set(model.map(x => x.split('@')[1])).size }) }}</span>
      <input v-if="model.length > 8" v-model="q" type="search" :placeholder="t('senders.find')" :aria-label="t('senders.findAria')" />
    </div>
    <div v-if="model.length" class="snd-list">
      <div v-for="g in groups" :key="g.domain" class="snd-group">
        <div class="snd-domain">
          <b>@{{ g.domain }}</b>
          <span v-if="g.whole" class="snd-whole" :title="t('senders.wholeDomainTitle')">{{ t('senders.wholeDomain') }}</span>
          <span v-if="g.items.length > 1" class="hint">{{ g.items.length }}</span>
        </div>
        <div class="snd-chips">
          <span v-for="x in g.items" :key="x.value" class="snd-chip" :class="{ whole: x.whole }" :title="x.value">{{ x.label }}<button type="button" :aria-label="t('senders.removeOne', { addr: x.value })" :title="t('senders.removeOne', { addr: x.value })" @click="remove(x.value)">×</button></span>
        </div>
        <button type="button" class="link danger-text snd-del" :title="t('senders.removeDomainTitle', { domain: g.domain })" @click="removeDomain(g.domain)">{{ t('senders.remove') }}</button>
      </div>
      <p v-if="!groups.length" class="hint" style="margin:6px 0">{{ t('senders.notFound') }}</p>
    </div>
    <p v-else class="hint" style="margin:0">{{ t('senders.empty') }}</p>
  </div>
</template>

<style scoped>
.snd{display:flex;flex-direction:column;gap:8px;font-weight:400}
.snd-add{display:flex;gap:8px}
.snd-add input,.snd-head input{flex:1;min-width:0;height:34px;border:1px solid var(--line);border-radius:4px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit;font-size:13.5px}
.snd-head{display:flex;align-items:center;gap:10px}
.snd-head input{max-width:200px;height:30px}
.snd-list{max-height:340px;overflow:auto;border:1px solid var(--line);border-radius:4px;padding:2px 10px}
.snd-group{display:flex;align-items:flex-start;gap:10px;padding:6px 0;border-bottom:1px solid var(--line)}
.snd-group:last-child{border-bottom:0}
.snd-domain{display:flex;align-items:center;gap:6px;font-size:13px;width:190px;flex:none;min-height:24px}
.snd-domain b{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.snd-domain b{font-weight:600;color:var(--ink)}
.snd-whole{font-size:11px;padding:0 7px;border-radius:999px;background:var(--accent-soft);color:var(--accent)}
.snd-chips{display:flex;flex-wrap:wrap;gap:5px;flex:1;min-width:0}
.snd-del{margin-left:auto;flex:none;line-height:24px}
.snd-chip{display:inline-flex;align-items:center;gap:2px;border:1px solid var(--line);border-radius:999px;padding:1px 4px 1px 9px;font-size:12.5px;color:var(--ink);background:var(--surface)}
.snd-chip.whole{border-color:var(--accent);color:var(--accent)}
.snd-chip button{border:0;background:none;color:var(--ink-3);cursor:pointer;font-size:15px;line-height:1;padding:0 3px;border-radius:50%}
.snd-chip button:hover{color:var(--danger);background:var(--surface-2)}
.grow{flex:1}
</style>
