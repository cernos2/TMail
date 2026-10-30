<script setup lang="ts">
// Access for AI agents over MCP: personal tokens with limits (mailboxes, changing mail, sending).
import { ref, computed, onMounted } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { confirmDialog, copyText } from '~/composables/dialogs'
import { t, fmtDate } from '~/composables/i18n'

type Tok = { id: number, name: string, prefix: string, canSend: boolean, canModify: boolean, mailboxes: string[], created: string, lastUsed: string | null, expires: string | null }
const tokens = ref<Tok[]>([])
const endpoint = ref('')
const form = ref({ name: '', canModify: false, canSend: false, all: true, mailboxes: [] as string[], days: 365 })
const created = ref<{ token: string, endpoint: string } | null>(null)
const busy = ref(false)
const loading = ref(true)

async function load() {
  loading.value = true
  try { const r = await api<{ tokens: Tok[], endpoint: string }>('/api/mcp/tokens'); tokens.value = r.tokens; endpoint.value = r.endpoint } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { loading.value = false }
}
onMounted(load)

async function create() {
  if (form.value.canSend && !await confirmDialog(t('mcp.sendConfirm'),
    t('mcp.sendConfirmText'), { ok: t('mcp.allow'), danger: true })) return
  busy.value = true
  try {
    const f = form.value
    created.value = await api('/api/mcp/tokens', { method: 'POST', body: { name: f.name, canModify: f.canModify, canSend: f.canSend, mailboxes: f.all ? [] : f.mailboxes, days: f.days } })
    form.value = { name: '', canModify: false, canSend: false, all: true, mailboxes: [], days: 365 }
    load()
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
async function revoke(tok: Tok) {
  if (!await confirmDialog(t('mcp.revokeConfirm', { name: tok.name }), t('mcp.revokeText'), { ok: t('mcp.revoke'), danger: true })) return
  busy.value = true
  try { await api(`/api/mcp/tokens/${tok.id}`, { method: 'DELETE' }); await load() } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
const fmt = (d: string | null) => d ? fmtDate(d, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'
const cmdClaude = computed(() => created.value ? `claude mcp add --transport http tmail ${created.value.endpoint} --header "Authorization: Bearer ${created.value.token}"` : '')
const jsonConfig = computed(() => created.value ? JSON.stringify({ mcpServers: { tmail: { type: 'http', url: created.value.endpoint, headers: { Authorization: `Bearer ${created.value.token}` } } } }, null, 2) : '')
async function copy(text: string) { await copyText(text); toast(t('common.copied')) }
</script>

<template>
  <div class="mcp veil-host">
    <LoadingVeil :show="loading || busy" :text="busy ? t('common.saving') : t('mcp.loading')" />
    <p class="hint">{{ t('mcp.intro') }}</p>

    <div v-if="created" class="mcp-new">
      <b>{{ t('mcp.tokenCreated') }}</b>
      <div class="mcp-code"><code>{{ created.token }}</code><button class="btn sm" @click="copy(created.token)"><AppIcon name="copy" />{{ t('common.copy') }}</button></div>
      <span class="hint">{{ t('mcp.claudeCode') }}</span>
      <div class="mcp-code"><code>{{ cmdClaude }}</code><button class="btn sm" @click="copy(cmdClaude)"><AppIcon name="copy" /></button></div>
      <span class="hint">{{ t('mcp.otherClients') }}</span>
      <div class="mcp-code"><pre>{{ jsonConfig }}</pre><button class="btn sm" @click="copy(jsonConfig)"><AppIcon name="copy" /></button></div>
      <button class="link" @click="created = null">{{ t('common.done') }}</button>
    </div>

    <form v-else class="mcp-form" @submit.prevent="create">
      <label class="f">{{ t('mcp.name') }}<input v-model="form.name" type="text" maxlength="80" :placeholder="t('mcp.namePlaceholder')" required /></label>
      <div class="f">{{ t('mcp.mailboxes') }}
        <label class="chk"><input v-model="form.all" type="checkbox" /> {{ t('mcp.allMine') }}</label>
        <template v-if="!form.all">
          <label v-for="a in mail.me?.accounts || []" :key="a.address" class="chk"><input v-model="form.mailboxes" type="checkbox" :value="a.address" /> {{ a.address }}</label>
        </template>
      </div>
      <div class="f">{{ t('mcp.permissions') }}
        <label class="chk"><input type="checkbox" checked disabled /> {{ t('mcp.permRead') }}</label>
        <label class="chk"><input v-model="form.canModify" type="checkbox" /> {{ t('mcp.permModify') }}</label>
        <label class="chk danger"><input v-model="form.canSend" type="checkbox" /> {{ t('mcp.permSend') }}</label>
      </div>
      <label class="f">{{ t('mcp.expiry') }}
        <select v-model.number="form.days"><option :value="30">{{ t('mcp.days', { n: 30 }) }}</option><option :value="90">{{ t('mcp.days', { n: 90 }) }}</option><option :value="365">{{ t('mcp.oneYear') }}</option><option :value="0">{{ t('mcp.noExpiry') }}</option></select></label>
      <div><button class="btn primary" :disabled="busy || !form.name.trim() || (!form.all && !form.mailboxes.length)"><AppIcon name="plus" />{{ t('mcp.createToken') }}</button></div>
    </form>

    <div class="mcp-list">
      <b>{{ t('mcp.issued') }}</b>
      <div v-for="tok in tokens" :key="tok.id" class="mcp-item">
        <div class="mcp-main">
          <span><b>{{ tok.name }}</b> <code class="dim">{{ tok.prefix }}…</code></span>
          <span class="hint">{{ tok.mailboxes.length ? tok.mailboxes.join(', ') : t('mcp.allMailboxes') }} · {{ tok.canSend ? t('mcp.canSend') : tok.canModify ? t('mcp.canModify') : t('mcp.readOnly') }}
            · {{ t('mcp.createdAt', { date: fmt(tok.created) }) }} · {{ t('mcp.lastUsed', { date: fmt(tok.lastUsed) }) }}{{ tok.expires ? ' · ' + t('mcp.until', { date: fmt(tok.expires) }) : '' }}</span>
        </div>
        <button class="link danger-text" @click="revoke(tok)">{{ t('mcp.revokeLink') }}</button>
      </div>
      <p v-if="!tokens.length" class="hint">{{ t('mcp.none') }}</p>
      <p class="hint">{{ t('mcp.endpoint') }} <code>{{ endpoint }}</code></p>
    </div>
  </div>
</template>

<style scoped>
.mcp{display:flex;flex-direction:column;gap:16px}
.mcp-form{display:flex;flex-direction:column;gap:12px}
.f{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--ink-2);font-weight:500}
.f input[type=text],.f select{height:36px;border:1px solid var(--line);border-radius:4.5px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit;font-size:14px;max-width:420px}
.chk{display:flex;align-items:flex-start;gap:8px;font-weight:400;color:var(--ink);font-size:13.5px}
.chk.danger{color:var(--danger)}
.mcp-new{display:flex;flex-direction:column;gap:8px;padding:14px;border:1px solid var(--accent);border-radius:6px;background:var(--accent-soft, var(--surface-2))}
.mcp-code{display:flex;align-items:flex-start;gap:8px}
.mcp-code code,.mcp-code pre{flex:1;min-width:0;margin:0;padding:8px 10px;background:var(--surface);border:1px solid var(--line);border-radius:4px;font-family:var(--f-mono);font-size:12px;overflow-x:auto;white-space:pre;word-break:normal}
.mcp-list{display:flex;flex-direction:column;gap:6px}
.mcp-item{display:flex;align-items:center;gap:12px;padding:10px 0;border-top:1px solid var(--line)}
.mcp-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:2px;font-size:14px}
.dim{color:var(--ink-3);font-size:12px}
</style>
