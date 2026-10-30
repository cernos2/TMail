<script setup lang="ts">
// Loggi AI assistant: connection status and every prompt the assistant uses, editable here.
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { adminApi, dateTime } from '~/composables/admin'
import { toast } from '~/composables/toast'
import { confirmDialog } from '~/composables/dialogs'
import { t, num } from '~/composables/i18n'
import { aiName } from '~/composables/ai'

type Prompt = { key: string, title: string, description: string, system: string, template: string, model: string,
  temperature: number, maxTokens: number, enabled: boolean, updatedAt?: string, updatedBy?: string }
const status = ref<any>(null)
const prompts = ref<Prompt[]>([])
const saved = ref<Record<string, string>>({})
const error = ref('')
const loading = ref(false)
const busy = ref('')
const open = ref<string | null>(null)
const tests = ref<Record<string, { letter: string, instructions: string, draft: string, result?: string, request?: string, ms?: number, err?: string, running?: boolean }>>({})

const PLACEHOLDERS = computed<[string, string][]>(() => [
  ['{letter}', t('admin.ai.phLetter')], ['{thread}', t('admin.ai.phThread')], ['{from}', t('admin.ai.phFrom')],
  ['{subject}', t('admin.ai.phSubject')], ['{to}', t('admin.ai.phTo')], ['{instructions}', t('admin.ai.phInstructions')],
  ['{draft}', t('admin.ai.phDraft')], ['{user}', t('admin.ai.phUser')], ['{mailbox}', t('admin.ai.phMailbox')],
])

// Which API the assistant uses: a local model, OpenAI, OpenRouter or any OpenAI-compatible one.
type Provider = { kind: string, name: string, url: string, model: string, asrUrl: string, asrModel: string }
const KINDS = computed(() => [
  { value: 'custom', label: t('admin.ai.kindCustom') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'openrouter', label: t('admin.ai.kindOpenrouter') },
  { value: 'loggi', label: 'Loggi' },
])
const prov = ref<Provider | null>(null)
const provSaved = ref('')
const provInfo = ref<{ hasKey: boolean, hasAsrKey: boolean, fromEnv: boolean, presets: Record<string, Provider> } | null>(null)
const key = ref('')
const asrKey = ref('')
const resetModels = ref(true)
async function loadProvider() {
  try {
    const r = await adminApi<any>('/ai/provider')
    prov.value = { ...r.provider }
    provSaved.value = JSON.stringify(r.provider)
    provInfo.value = r
    key.value = ''; asrKey.value = ''
  } catch (e: any) { error.value = e.message }
}
const provDirty = computed(() => !!prov.value && (JSON.stringify(prov.value) !== provSaved.value || !!key.value || !!asrKey.value))
const urlChanged = computed(() => !!prov.value && JSON.parse(provSaved.value || '{}').url !== prov.value.url)
function pickKind(k: string) {
  const p = prov.value
  if (!p) return
  p.kind = k
  const pre = provInfo.value?.presets?.[k]
  if (pre) { p.url = pre.url; p.model = pre.model; p.asrModel = pre.asrModel || '' }
}
async function saveProvider() {
  const p = prov.value
  if (!p) return
  busy.value = 'provider'
  try {
    await adminApi('/ai/provider', { method: 'PUT', body: { ...p, key: key.value ? key.value : undefined, asrKey: asrKey.value ? asrKey.value : undefined, resetModels: urlChanged.value && resetModels.value } })
    toast(t('admin.ai.providerSaved'))
    await loadProvider()
    await load()
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = '' }
}
onMounted(loadProvider)

async function load() {
  loading.value = true; error.value = ''
  try {
    const r = await adminApi<{ status: any, prompts: Prompt[] }>('/ai')
    status.value = r.status
    prompts.value = r.prompts
    saved.value = Object.fromEntries(r.prompts.map(p => [p.key, JSON.stringify(p)]))
    for (const p of r.prompts) tests.value[p.key] ||= { letter: t('admin.ai.sampleLetter'), instructions: '', draft: '' }
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}
const crmS = ref<any>(null)
const days = ref(90)
let poll: ReturnType<typeof setInterval> | null = null
async function loadCrm() { try { crmS.value = await adminApi('/crm') } catch {} }
async function backfill() {
  try { await adminApi('/crm/backfill', { method: 'POST', body: { days: days.value } }); toast(t('admin.ai.backfillStarted')); loadCrm() }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}
onMounted(() => { load(); loadCrm(); poll = setInterval(() => { if (crmS.value?.job?.running) loadCrm() }, 3000) })
onBeforeUnmount(() => { if (poll) clearInterval(poll) })
const dirty = (p: Prompt) => saved.value[p.key] !== JSON.stringify(p)

async function save(p: Prompt) {
  busy.value = p.key
  try {
    await adminApi(`/ai/prompts/${p.key}`, { method: 'PUT', body: p })
    p.updatedAt = new Date().toISOString()
    saved.value[p.key] = JSON.stringify(p)
    toast(t('admin.ai.promptSaved'))
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = '' }
}
async function test(p: Prompt) {
  const tc = tests.value[p.key]
  tc.running = true; tc.err = ''; tc.result = ''
  try {
    const r = await adminApi<{ text: string, request: string, ms: number }>('/ai/test', { method: 'POST', timeout: 160000,
      body: { prompt: p, input: { from: t('admin.ai.sampleFrom'), subject: t('admin.ai.sampleSubject'), letter: tc.letter, instructions: tc.instructions, draft: tc.draft } } })
    Object.assign(tc, { result: r.text, request: r.request, ms: r.ms })
  } catch (e: any) { tc.err = e.message } finally { tc.running = false }
}

// Access tokens of AI agents (MCP) of all users.
type McpTok = { id: number, name: string, prefix: string, login: string, canSend: boolean, canModify: boolean, mailboxes: string[], created: string, lastUsed: string | null }
const mcpTokens = ref<McpTok[]>([])
async function loadMcp() { try { mcpTokens.value = (await adminApi<{ tokens: McpTok[] }>('/mcp-tokens')).tokens } catch {} }
onMounted(loadMcp)
async function revokeMcp(tok: McpTok) {
  if (!await confirmDialog(t('admin.ai.revokeConfirm', { name: tok.name, login: tok.login }), t('admin.ai.revokeConfirmText'), { ok: t('admin.ai.revoke'), danger: true })) return
  try { await adminApi(`/mcp-tokens/${tok.id}`, { method: 'DELETE' }); loadMcp() } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
</script>

<template>
  <div class="adm-page veil-host">
    <LoadingVeil :show="loading || !!busy" :text="busy ? t('common.saving') : t('admin.ai.checkingConnection')" />
    <AdminHeader :title="t('admin.ai.title')" :desc="t('admin.ai.desc')">
      <button class="btn" :disabled="loading" @click="load"><AppIcon name="refresh" />{{ loading ? t('admin.ai.checking') : t('common.refresh') }}</button>
    </AdminHeader>
    <div v-if="error" class="notice danger">{{ error }}</div>

    <section v-if="prov" class="card">
      <div class="card-head"><h2>{{ t('admin.ai.connection') }}</h2><span v-if="provInfo?.fromEnv" class="hint">{{ t('admin.ai.fromEnv') }}</span></div>
      <div class="card-body form ai-form">
        <div class="row2">
          <label class="f">{{ t('admin.ai.service') }}<AppSelect :model-value="prov.kind" :options="KINDS" :aria-label="t('admin.ai.serviceAria')" @update:model-value="pickKind" /></label>
          <label class="f">{{ t('admin.ai.assistantName') }}<input v-model="prov.name" type="text" placeholder="Loggi" /></label>
        </div>
        <div class="row2">
          <label class="f">{{ t('admin.ai.apiUrl') }}<input v-model="prov.url" type="text" inputmode="url" placeholder="https://api.example.com" spellcheck="false" /></label>
          <label class="f">{{ t('admin.ai.apiKey') }}
            <input v-model="key" type="password" autocomplete="new-password" spellcheck="false"
                   :placeholder="provInfo?.hasKey ? t('admin.ai.keySavedPlaceholder') : (prov.kind === 'custom' ? t('admin.ai.keyNotNeeded') : 'sk-…')" /></label>
        </div>
        <label class="f">{{ t('admin.ai.defaultModel') }}
          <input v-model="prov.model" type="text" list="ai-models" spellcheck="false" :placeholder="t('admin.ai.defaultModelPlaceholder')" /></label>
        <datalist id="ai-models"><option v-for="m in status?.models || []" :key="m" :value="m" /></datalist>
        <p v-if="prov.kind === 'openrouter'" class="hint">{{ t('admin.ai.openrouterKeyAt') }} <a href="https://openrouter.ai/keys" target="_blank" rel="noopener">openrouter.ai/keys</a>, {{ t('admin.ai.openrouterModelsAt') }} <a href="https://openrouter.ai/models" target="_blank" rel="noopener">openrouter.ai/models</a>. {{ t('admin.ai.openrouterVision') }}</p>
        <details class="ai-test" :open="prov.kind === 'openrouter' || !!prov.asrUrl">
          <summary>{{ t('admin.ai.asr') }}</summary>
          <p class="hint">{{ prov.kind === 'openrouter' ? t('admin.ai.asrOpenrouter') : t('admin.ai.asrHint') }}</p>
          <div class="row3">
            <label class="f">{{ t('admin.ai.apiUrl') }}<input v-model="prov.asrUrl" type="text" inputmode="url" :placeholder="t('admin.ai.asrUrlPlaceholder')" spellcheck="false" /></label>
            <label class="f">{{ t('admin.ai.key') }}<input v-model="asrKey" type="password" autocomplete="new-password" :placeholder="provInfo?.hasAsrKey ? t('admin.ai.keySaved') : ''" /></label>
            <label class="f">{{ t('admin.ai.model') }}<input v-model="prov.asrModel" type="text" placeholder="whisper-1" spellcheck="false" /></label>
          </div>
        </details>
        <label v-if="urlChanged" class="check"><input v-model="resetModels" type="checkbox" />{{ t('admin.ai.resetModels') }}</label>
        <div class="ai-actions"><span class="grow"></span>
          <button class="btn primary" :disabled="!provDirty || busy === 'provider'" @click="saveProvider">{{ busy === 'provider' ? t('common.saving') : provDirty ? t('admin.ai.saveAndCheck') : t('common.saved') }}</button></div>
      </div>
      <div class="kv">
        <div>{{ t('admin.ai.state') }}</div>
        <div>
          <span v-if="!status" class="hint">{{ t('admin.ai.checkingLower') }}</span>
          <span v-else-if="!status.configured" class="badge bad">{{ t('admin.ai.notConfigured') }}</span>
          <span v-else-if="status.reachable" class="badge ok"><AppIcon name="check" />{{ t('admin.ai.working', { ms: num(status.ms) }) }}</span>
          <span v-else class="badge bad">{{ status.error ? t('admin.ai.unreachableError', { error: status.error }) : t('admin.ai.unreachable') }}</span>
        </div>
        <template v-if="status?.host"><div>{{ t('admin.ai.server') }}</div><div>{{ status.host }}</div></template>
        <template v-if="status?.models?.length"><div>{{ t('admin.ai.modelsAvailable') }}</div><div>{{ status.models.length > 12 ? status.models.length : status.models.join(', ') }}</div></template>
        <template v-if="status?.configured && !status.secure"><div>{{ t('admin.ai.channel') }}</div><div class="warn-text">{{ t('admin.ai.insecure') }}</div></template>
      </div>
    </section>

    <section v-if="crmS" class="card">
      <div class="card-head"><h2>{{ t('admin.ai.crmTitle') }}</h2></div>
      <div class="card-body" style="display:flex;flex-direction:column;gap:12px">
        <div class="kv">
          <div>{{ t('admin.ai.crmCollected') }}</div><div>{{ t('admin.ai.crmStats', { companies: crmS.companies, people: crmS.people, files: crmS.files, analysed: crmS.analysed }) }}</div>
          <div>{{ t('admin.ai.crmHow') }}</div><div>{{ t('admin.ai.crmHowText', { ai: aiName() }) }}</div>
        </div>
        <div v-if="crmS.job?.running" class="usage">
          <span>{{ crmS.job.errors ? t('admin.ai.backfillProgressErrors', { done: num(crmS.job.done), total: num(crmS.job.total), found: num(crmS.job.found), errors: num(crmS.job.errors) }) : t('admin.ai.backfillProgress', { done: num(crmS.job.done), total: num(crmS.job.total), found: num(crmS.job.found) }) }}</span>
          <div class="meter"><i :style="{ width: (crmS.job.total ? Math.round(crmS.job.done / crmS.job.total * 100) : 0) + '%' }"></i></div>
          <span class="hint">{{ crmS.job.current }}</span>
        </div>
        <div v-else style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
          <span>{{ t('admin.ai.backfillBefore') }}</span>
          <input v-model.number="days" type="number" min="1" max="730" style="width:80px;height:34px;border:1px solid var(--line);border-radius:4px;padding:0 8px;background:var(--surface-2);color:var(--ink)" />
          <span>{{ t('admin.ai.backfillAfter') }}</span>
          <button class="btn" @click="backfill"><AppIcon name="sparkle" />{{ t('admin.ai.run') }}</button>
          <span v-if="crmS.job?.total" class="hint">{{ t('admin.ai.backfillLast', { done: crmS.job.done, found: crmS.job.found }) }}</span>
        </div>
        <span class="hint">{{ t('admin.ai.backfillHint') }}</span>
      </div>
    </section>

    <section class="card">
      <div class="card-head"><h2>{{ t('admin.ai.mcpTitle') }}</h2><span class="hint">{{ t('admin.ai.mcpHint') }}</span></div>
      <div v-for="tok in mcpTokens" :key="tok.id" class="mcp-adm">
        <div class="mcp-adm-main"><span><b>{{ tok.login }}</b> · {{ tok.name }} <code>{{ tok.prefix }}…</code></span>
          <span class="hint">{{ tok.mailboxes.length ? tok.mailboxes.join(', ') : t('admin.ai.mcpAllMailboxes') }} · {{ tok.canSend ? t('admin.ai.mcpCanSend') : tok.canModify ? t('admin.ai.mcpCanModify') : t('admin.ai.mcpReadOnly') }} · {{ t('admin.ai.mcpLastUsed', { when: tok.lastUsed ? dateTime(tok.lastUsed) : '—' }) }}</span></div>
        <button class="link danger-text" @click="revokeMcp(tok)">{{ t('admin.ai.revokeLower') }}</button>
      </div>
      <p v-if="!mcpTokens.length" class="card-body hint">{{ t('admin.ai.mcpNone') }}</p>
    </section>

    <section v-for="p in prompts" :key="p.key" class="card ai-card">
      <div class="card-head ai-head" role="button" tabindex="0" @click="open = open === p.key ? null : p.key" @keydown.enter="open = open === p.key ? null : p.key">
        <div><h2>{{ p.title }} <span class="hint mono">{{ p.key }}</span></h2><p class="hint">{{ p.description }}</p></div>
        <span class="badge" :class="p.enabled ? 'ok' : ''">{{ p.enabled ? t('admin.ai.enabled') : t('admin.ai.disabled') }}</span>
        <AppIcon :name="open === p.key ? 'chevUp' : 'chev'" />
      </div>
      <div v-if="open === p.key" class="card-body form ai-form">
        <div class="row2">
          <label class="f">{{ t('admin.ai.buttonTitle') }}<input v-model="p.title" type="text" /></label>
          <label class="f">{{ t('admin.ai.description') }}<input v-model="p.description" type="text" /></label>
        </div>
        <label class="f">{{ t('admin.ai.systemPrompt') }}
          <textarea v-model="p.system" rows="9" spellcheck="true"></textarea></label>
        <label class="f">{{ t('admin.ai.template') }}
          <textarea v-model="p.template" rows="9" spellcheck="true"></textarea></label>
        <div class="ph">
          <span class="hint">{{ t('admin.ai.placeholders') }}</span>
          <code v-for="[k, d] in PLACEHOLDERS" :key="k" :title="d">{{ k }}<small>{{ d }}</small></code>
        </div>
        <div class="row3">
          <label class="f">{{ t('admin.ai.model') }}
            <input v-model="p.model" type="text" list="ai-models" spellcheck="false" :placeholder="t('admin.ai.modelDefault', { model: prov?.model || '—' })" /></label>
          <label class="f">{{ t('admin.ai.temperature') }}<input v-model.number="p.temperature" type="number" min="0" max="1.5" step="0.1" /></label>
          <label class="f">{{ t('admin.ai.maxTokens') }}<input v-model.number="p.maxTokens" type="number" min="50" max="8000" step="50" /></label>
        </div>
        <label class="check"><input v-model="p.enabled" type="checkbox" />{{ t('admin.ai.showToStaff') }}</label>
        <div class="ai-actions">
          <span v-if="p.updatedAt" class="hint">{{ t('admin.ai.changedAt', { when: dateTime(p.updatedAt) }) }}{{ p.updatedBy ? ' · ' + p.updatedBy : '' }}</span>
          <span class="grow"></span>
          <button class="btn primary" :disabled="!dirty(p) || busy === p.key" @click="save(p)">{{ busy === p.key ? t('common.saving') : dirty(p) ? t('common.save') : t('common.saved') }}</button>
        </div>

        <details class="ai-test">
          <summary>{{ t('admin.ai.testTitle') }}</summary>
          <label class="f">{{ t('admin.ai.testLetter') }}<textarea v-model="tests[p.key].letter" rows="5"></textarea></label>
          <div class="row2">
            <label class="f">{{ t('admin.ai.testInstructions') }}<input v-model="tests[p.key].instructions" type="text" :placeholder="t('admin.ai.testInstructionsPlaceholder')" /></label>
            <label class="f">{{ t('admin.ai.testDraft') }}<input v-model="tests[p.key].draft" type="text" /></label>
          </div>
          <div><button class="btn" :disabled="tests[p.key].running" @click="test(p)"><AppIcon name="sparkle" />{{ tests[p.key].running ? t('admin.ai.thinking', { ai: aiName() }) : t('admin.ai.run') }}</button></div>
          <p v-if="tests[p.key].err" class="err">{{ tests[p.key].err }}</p>
          <template v-if="tests[p.key].result">
            <div class="ai-box"><b><AppIcon name="sparkle" />{{ t('admin.ai.modelAnswer', { ms: num(tests[p.key].ms || 0) }) }}</b>{{ tests[p.key].result }}</div>
            <details><summary class="hint">{{ t('admin.ai.sentToModel') }}</summary><pre class="ai-req">{{ tests[p.key].request }}</pre></details>
          </template>
        </details>
      </div>
    </section>
  </div>
</template>

<style scoped>
.ai-head{cursor:pointer;align-items:flex-start;gap:12px}
.ai-head > div{flex:1}
.ai-head h2{display:flex;align-items:center;gap:8px}
.ai-head p{margin:4px 0 0}
.mono{font-family:var(--f-mono);font-weight:400}
.ai-form{display:flex;flex-direction:column;gap:14px}
.ai-form textarea{min-height:160px;font-family:var(--f-mono);font-size:13px}
.row3{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}
@media (max-width:760px){.row3{grid-template-columns:1fr}}
.ph{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
.ph code{font:12px var(--f-mono);background:var(--surface-2);border:1px solid var(--line);border-radius:3px;padding:3px 7px;display:inline-flex;gap:6px;align-items:baseline}
.ph code small{font-family:inherit;color:var(--ink-3);font-size:11px}
.check{display:flex;align-items:center;gap:8px}
.ai-actions{display:flex;align-items:center;gap:10px}
.ai-test{border-top:1px solid var(--line);padding-top:12px;display:flex;flex-direction:column;gap:10px}
.ai-test[open]{display:flex}
.ai-test summary{cursor:pointer;font-weight:500;color:var(--accent)}
.ai-test > *+*{margin-top:10px}
.ai-req{white-space:pre-wrap;font:12px/1.5 var(--f-mono);background:var(--surface-2);border-radius:4px;padding:10px;max-height:300px;overflow:auto}
.warn-text{color:var(--warn,#b7791f)}
.mcp-adm{display:flex;align-items:center;gap:12px;padding:10px 16px;border-top:1px solid var(--line)}
.mcp-adm-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px;font-size:14px}
</style>
