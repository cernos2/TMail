<script setup lang="ts">
// OpenPGP: a key for each of the user's mailboxes (made here or brought from another program) and
// the correspondents' public keys that letters are encrypted with.
import { ref, computed, onMounted } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { confirmDialog } from '~/composables/dialogs'
import { forgetPgpChecks } from '~/composables/pgp'
import { t, fmtDate } from '~/composables/i18n'

type Key = { mailbox: string, fingerprint: string, signDefault: boolean, created: string, createdBy: string }
type Contact = { email: string, fingerprint: string, source: string, updated: string }
const keys = ref<Key[]>([])
const contacts = ref<Contact[]>([])
const loading = ref(true)
const busy = ref('')
const importing = ref<{ mailbox: string, armored: string, passphrase: string } | null>(null)
const adding = ref<string | null>(null)

async function load() {
  loading.value = true
  try { const r = await api<{ keys: Key[], contacts: Contact[] }>('/api/pgp'); keys.value = r.keys; contacts.value = r.contacts }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { loading.value = false }
}
onMounted(load)
const boxes = computed(() => (mail.me?.accounts || []).filter(a => a.canSend).map(a => ({ ...a, key: keys.value.find(k => k.mailbox === a.address) })))
const fp = (f: string) => f.replace(/(.{4})/g, '$1 ').trim()
const date = (d: string) => fmtDate(d, { day: 'numeric', month: 'short', year: 'numeric' })

async function generate(mailbox: string) {
  busy.value = t('pgp.generating')
  try { await api('/api/pgp/keys', { method: 'POST', body: { mailbox, action: 'generate' } }); toast(t('pgp.created', { mailbox })); forgetPgpChecks(); load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = '' }
}
async function doImport() {
  const x = importing.value
  if (!x) return
  busy.value = t('common.saving')
  try { await api('/api/pgp/keys', { method: 'POST', body: { mailbox: x.mailbox, action: 'import', armored: x.armored, passphrase: x.passphrase } }); importing.value = null; toast(t('pgp.imported')); forgetPgpChecks(); load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = '' }
}
async function remove(k: Key) {
  if (!await confirmDialog(t('pgp.deleteConfirm', { mailbox: k.mailbox }), t('pgp.deleteText'), { ok: t('common.delete'), danger: true })) return
  try { await api('/api/pgp/keys', { method: 'DELETE', query: { mailbox: k.mailbox } }); forgetPgpChecks(); load() } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function setSign(k: Key, v: boolean) {
  k.signDefault = v
  try { await api('/api/pgp/keys', { method: 'PUT', body: { mailbox: k.mailbox, signDefault: v } }); forgetPgpChecks() } catch (e: any) { k.signDefault = !v; toast(e.message, { kind: 'err' }) }
}
function download(email: string) {
  const a = document.createElement('a'); a.href = `/api/pgp/public?email=${encodeURIComponent(email)}`; a.download = ''; a.click()
}
async function addContact() {
  if (!adding.value?.trim()) return
  busy.value = t('common.saving')
  try { const r = await api<{ emails: string[] }>('/api/pgp/contacts', { method: 'POST', body: { armored: adding.value } }); adding.value = null; toast(t('pgp.keySaved', { emails: r.emails.join(', ') })); forgetPgpChecks(); load() }
  catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = '' }
}
async function removeContact(c: Contact) {
  if (!await confirmDialog(t('pgp.contactDeleteConfirm', { email: c.email }), '', { ok: t('common.delete'), danger: true })) return
  try { await api('/api/pgp/contacts', { method: 'DELETE', query: { email: c.email } }); forgetPgpChecks(); load() } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function readFile(e: Event, set: (v: string) => void) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) set(await f.text())
}
const SOURCES: Record<string, () => string> = { manual: () => t('pgp.srcManual'), attachment: () => t('pgp.srcAttachment'), autocrypt: () => t('pgp.srcAutocrypt') }
</script>

<template>
  <div class="pgpx veil-host">
    <LoadingVeil :show="loading || !!busy" :text="busy || t('pgp.loading')" />
    <p class="hint" style="margin:0">{{ t('pgp.intro') }}</p>

    <section class="pg-sec">
      <h3><AppIcon name="key" />{{ t('pgp.myKeys') }}</h3>
      <div v-for="b in boxes" :key="b.address" class="pg-box">
        <span class="pg-ico" :class="{ on: b.key }"><AppIcon :name="b.key ? 'lock' : 'unlock'" /></span>
        <div class="pg-main">
          <b>{{ b.address }}</b>
          <template v-if="b.key">
            <code class="pg-fp">{{ fp(b.key.fingerprint) }}</code>
            <span class="hint">{{ t('pgp.createdBy', { date: date(b.key.created), login: b.key.createdBy }) }}</span>
            <label class="check"><input type="checkbox" :checked="b.key.signDefault" @change="setSign(b.key!, ($event.target as HTMLInputElement).checked)" />{{ t('pgp.signDefault') }}</label>
          </template>
          <span v-else class="hint">{{ t('pgp.noKey') }}</span>
        </div>
        <div class="pg-act">
          <template v-if="b.key">
            <button class="btn sm" @click="download(b.address)"><AppIcon name="download" />{{ t('pgp.publicKey') }}</button>
            <button class="btn sm danger-text" @click="remove(b.key!)"><AppIcon name="trash" /></button>
          </template>
          <template v-else>
            <button class="btn sm primary" @click="generate(b.address)"><AppIcon name="plus" />{{ t('pgp.create') }}</button>
            <button class="btn sm" @click="importing = { mailbox: b.address, armored: '', passphrase: '' }"><AppIcon name="upload" />{{ t('pgp.importOwn') }}</button>
          </template>
        </div>
      </div>
    </section>

    <section class="pg-sec">
      <h3><AppIcon name="users" />{{ t('pgp.contacts') }}<span class="grow"></span>
        <button class="btn sm" @click="adding = ''"><AppIcon name="plus" />{{ t('pgp.addContact') }}</button></h3>
      <p class="hint" style="margin:0">{{ t('pgp.contactsHint') }}</p>
      <div v-if="adding !== null" class="pg-add form">
        <textarea v-model="adding" rows="6" :placeholder="'-----BEGIN PGP PUBLIC KEY BLOCK-----'"></textarea>
        <div class="pg-row"><label class="btn sm"><AppIcon name="file" />{{ t('pgp.fromFile') }}<input type="file" accept=".asc,.pub,.gpg,.txt" hidden @change="readFile($event, v => adding = v)" /></label>
          <span class="grow"></span><button class="btn sm" @click="adding = null">{{ t('common.cancel') }}</button>
          <button class="btn sm primary" :disabled="!adding.trim()" @click="addContact">{{ t('pgp.saveKey') }}</button></div>
      </div>
      <div v-if="contacts.length" class="pg-list">
        <div v-for="c in contacts" :key="c.email" class="pg-contact">
          <div class="pg-main"><b>{{ c.email }}</b><code class="pg-fp">{{ fp(c.fingerprint) }}</code></div>
          <span class="hint">{{ SOURCES[c.source]?.() || c.source }} · {{ date(c.updated) }}</span>
          <button class="icon-btn" :title="t('pgp.publicKey')" :aria-label="t('pgp.publicKey')" @click="download(c.email)"><AppIcon name="download" /></button>
          <button class="icon-btn" :title="t('common.delete')" :aria-label="t('common.delete')" @click="removeContact(c)"><AppIcon name="trash" /></button>
        </div>
      </div>
      <p v-else-if="!loading" class="hint pg-empty">{{ t('pgp.noContacts') }}</p>
    </section>

    <AppModal v-if="importing" :title="t('pgp.importTitle', { mailbox: importing.mailbox })" width="560px" @close="importing = null">
      <div class="form">
        <label class="f">{{ t('pgp.privateKey') }}<textarea v-model="importing.armored" rows="8" placeholder="-----BEGIN PGP PRIVATE KEY BLOCK-----"></textarea></label>
        <label class="btn sm" style="align-self:flex-start"><AppIcon name="file" />{{ t('pgp.fromFile') }}<input type="file" accept=".asc,.key,.gpg,.txt" hidden @change="readFile($event, v => importing!.armored = v)" /></label>
        <label class="f">{{ t('pgp.passphrase') }}<input v-model="importing.passphrase" type="password" autocomplete="off" /></label>
        <p class="hint" style="margin:0">{{ t('pgp.importHint') }}</p>
      </div>
      <template #foot>
        <button class="btn" @click="importing = null">{{ t('common.cancel') }}</button>
        <button class="btn primary" :disabled="!importing.armored.trim()" @click="doImport">{{ t('pgp.importBtn') }}</button>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.pgpx{display:flex;flex-direction:column;gap:16px;min-height:240px}
.pg-sec{display:flex;flex-direction:column;gap:8px}
.pg-sec h3{display:flex;align-items:center;gap:8px;margin:0;font-size:14px}
.pg-sec h3 > svg{width:16px;height:16px;color:var(--accent)}
.pg-box{display:flex;align-items:flex-start;gap:12px;padding:12px 14px;border:1px solid var(--line);border-radius:6px;background:var(--surface)}
.pg-ico{width:34px;height:34px;border-radius:50%;display:grid;place-items:center;background:var(--surface-3);color:var(--ink-3);flex:none}
.pg-ico.on{background:var(--ok-soft);color:var(--ok)}
.pg-ico svg{width:17px;height:17px}
.pg-main{display:flex;flex-direction:column;gap:3px;flex:1;min-width:0}
.pg-main .hint{font-size:12.5px}
.pg-fp{font-family:var(--f-mono);font-size:12px;color:var(--ink-2);word-break:break-all}
.pg-act{display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end}
.pg-list{display:flex;flex-direction:column;border:1px solid var(--line);border-radius:6px;overflow:hidden}
.pg-contact{display:flex;align-items:center;gap:10px;padding:8px 12px;border-top:1px solid var(--line);background:var(--surface)}
.pg-contact:first-child{border-top:0}
.pg-contact > .hint{font-size:12px;white-space:nowrap}
.pg-empty{margin:0;padding:14px;border:1px dashed var(--line);border-radius:6px;text-align:center}
.pg-add textarea{font-family:var(--f-mono);font-size:12px}
.pg-row{display:flex;gap:8px;align-items:center}
@media (max-width:700px){.pg-box{flex-wrap:wrap}.pg-contact{flex-wrap:wrap}}
</style>
