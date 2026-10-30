<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { adminApi, quotas, GB, bytes } from '~/composables/admin'
import { toast } from '~/composables/toast'
import { confirmDialog, showSecret } from '~/composables/dialogs'
import { t, fmtDate } from '~/composables/i18n'

const props = defineProps<{ mailbox: any | null, domains: any[], users: any[] }>()
const emit = defineEmits<{ close: [], saved: [] }>()
const isNew = !props.mailbox
const tab = ref('main')

const quotaPreset = (b: number) => quotas().some(q => q.bytes === b) ? String(b) : 'custom'
const form = reactive({
  localPart: '',
  domain: props.domains[0]?.name || '',
  name: props.mailbox?.name || '',
  quota: quotaPreset(props.mailbox?.quotaBytes ?? 5 * GB),
  customGb: props.mailbox ? Math.round((props.mailbox.quotaBytes || 0) / GB) : 5,
  active: props.mailbox?.active ?? true,
  signature: props.mailbox?.signature || '',
  template: props.mailbox?.template || '',
  autoPassword: true,
  password: '',
  grantUsers: [] as number[],
})
// Who works with this mailbox: userId → rights.
const access = reactive<Record<number, { canSend: boolean, personal: boolean } | undefined>>({})
for (const u of props.mailbox?.users || []) access[u.id] = { canSend: u.canSend, personal: u.personal }
const accessCount = computed(() => Object.values(access).filter(Boolean).length)
function toggleUser(id: number, on: boolean) {
  if (on) access[id] = { canSend: true, personal: false }
  else delete access[id]
}
function setPersonal(id: number) {
  const a = access[id]
  if (!a) return
  a.personal = !a.personal
  if (a.personal) a.canSend = true
}
const error = ref('')
const saving = ref(false)
const quotaBytes = computed(() => form.quota === 'custom' ? Math.max(0, Math.round(form.customGb * GB)) : Number(form.quota))
const clientRows = (addr: string): [string, string][] => [
  [t('common.login'), addr],
  [t('admin.mailboxes.incoming'), t('admin.mailboxes.serverPort', { proto: 'IMAP', host: location.hostname, port: 993 })],
  [t('admin.mailboxes.outgoing'), t('admin.mailboxes.serverPort', { proto: 'SMTP', host: location.hostname, port: 465 })],
]

async function save() {
  error.value = ''
  if (isNew && !/^[a-z0-9]([a-z0-9._+-]{0,62}[a-z0-9])?$/.test(form.localPart.trim().toLowerCase())) {
    error.value = t('admin.mailboxes.localPartInvalid'); tab.value = 'main'; return
  }
  if (isNew && !form.autoPassword && form.password.length < 10) { error.value = t('admin.users.passwordShort'); return }
  saving.value = true
  try {
    if (isNew) {
      const res = await adminApi('/mailboxes', { method: 'POST', body: {
        localPart: form.localPart.trim().toLowerCase(), domain: form.domain, name: form.name.trim(), quotaBytes: quotaBytes.value,
        password: form.autoPassword ? '' : form.password, grantUsers: form.grantUsers,
      } })
      toast(t('admin.mailboxes.created', { address: res.address }))
      if (res.password) showSecret({ title: t('admin.mailboxes.passwordFor', { address: res.address }), secret: res.password,
        note: t('admin.mailboxes.passwordNote'), rows: clientRows(res.address) })
    } else {
      await adminApi(`/mailboxes/${props.mailbox.id}`, { method: 'PATCH', body: {
        name: form.name.trim(), quotaBytes: quotaBytes.value, active: form.active, signature: form.signature, template: form.template,
      } })
      await adminApi(`/mailboxes/${props.mailbox.id}/users`, { method: 'PUT', body: Object.entries(access)
        .filter(([, v]) => v).map(([id, v]) => ({ userId: Number(id), canSend: v!.canSend, personal: v!.personal })) })
      toast(t('admin.users.changesSaved'))
    }
    emit('saved')
  } catch (e: any) { error.value = e.message } finally { saving.value = false }
}

// The current password is only a hash; a new one is shown in the settings card once.
const ownPass = ref('')
const shownPass = ref('')
async function resetPassword() {
  if (!await confirmDialog(t('admin.mailboxes.changePasswordConfirm', { address: props.mailbox.address }), t('admin.mailboxes.changePasswordText'), { ok: t('admin.mailboxes.changePassword') })) return
  try {
    const typed = ownPass.value
    const res = await adminApi(`/mailboxes/${props.mailbox.id}/password`, { method: 'POST', body: { password: typed } })
    shownPass.value = res.password || typed
    ownPass.value = ''
    toast(t('admin.mailboxes.passwordChanged'))
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
// ---------- deleting the mailbox with all its letters ----------
const delConfirm = ref('')
const delBusy = ref(false)
const deleted = computed(() => !!props.mailbox?.deletedAt)
const purgeDate = computed(() => props.mailbox?.purgeAfter ? fmtDate(props.mailbox.purgeAfter, { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' }) : '')
const typedOk = computed(() => delConfirm.value.trim().toLowerCase() === props.mailbox?.address)
async function deleteMailbox(now: boolean) {
  const addr = props.mailbox.address
  const ok = await confirmDialog(now ? t('admin.mailboxes.eraseNowConfirm', { address: addr }) : t('admin.mailboxes.deleteConfirm', { address: addr }),
    now ? t('admin.mailboxes.eraseNowText')
      : t('admin.mailboxes.deleteText'),
    { ok: now ? t('admin.mailboxes.eraseForever') : t('common.delete'), danger: true })
  if (!ok) return
  delBusy.value = true
  try {
    await adminApi(`/mailboxes/${props.mailbox.id}/delete`, { method: 'POST', body: { confirm: delConfirm.value, now } })
    toast(now ? t('admin.mailboxes.erased', { address: addr }) : t('admin.mailboxes.deleted', { address: addr }))
    emit('saved')
  } catch (e: any) { toast(e.message, { kind: 'err', ms: 9000 }) } finally { delBusy.value = false }
}
async function restoreMailbox() {
  try { await adminApi(`/mailboxes/${props.mailbox.id}/restore`, { method: 'POST' }); toast(t('admin.mailboxes.restored')); emit('saved') }
  catch (e: any) { toast(e.message, { kind: 'err' }) }
}

async function toggleActive() {
  if (form.active && !await confirmDialog(t('admin.mailboxes.disableConfirm', { address: props.mailbox.address }), t('admin.mailboxes.disableText'), { ok: t('admin.mailboxes.disable'), danger: true })) return
  form.active = !form.active
}
</script>

<template>
  <AppModal :title="isNew ? t('admin.mailboxes.newTitle') : props.mailbox.address" width="680px" :busy="saving ? t('common.saving') : delBusy ? t('admin.mailboxes.deleting') : ''" @close="emit('close')">
    <template v-if="!isNew" #tabs>
      <div class="tabs" role="tablist">
        <button :class="{ on: tab === 'main' }" @click="tab = 'main'">{{ t('admin.users.tabMain') }}</button>
        <button :class="{ on: tab === 'access' }" @click="tab = 'access'">{{ t('admin.mailboxes.tabAccess', { n: accessCount }) }}</button>
        <button :class="{ on: tab === 'sign' }" @click="tab = 'sign'">{{ t('admin.mailboxes.tabSign') }}</button>
        <button :class="{ on: tab === 'conn' }" @click="tab = 'conn'">{{ t('admin.mailboxes.tabClients') }}</button>
      </div>
    </template>

    <div v-if="tab === 'main'" class="form">
      <label v-if="isNew" class="f">{{ t('admin.mailboxes.address') }}
        <div class="addon">
          <input v-model="form.localPart" type="text" placeholder="ivanov" autocomplete="off" autocapitalize="none" spellcheck="false" style="flex:1" />
          <span style="padding:0 4px">@</span>
          <AppSelect v-model="form.domain" :options="props.domains.map(d => ({ value: d.name, label: d.name }))" bare :aria-label="t('admin.mailboxes.domain')" />
        </div>
      </label>
      <label class="f">{{ t('admin.mailboxes.senderName') }}<input v-model="form.name" type="text" :placeholder="t('admin.mailboxes.senderNamePlaceholder')" />
        <span class="help">{{ t('admin.mailboxes.senderNameHelp') }}</span></label>
      <div class="row2">
        <label class="f">{{ t('admin.mailboxes.quota') }}
          <AppSelect v-model="form.quota" :options="[...quotas().map(q => ({ value: String(q.bytes), label: q.label })), { value: 'custom', label: t('admin.mailboxes.quotaCustom') }]" :aria-label="t('admin.mailboxes.quota')" />
        </label>
        <label v-if="form.quota === 'custom'" class="f">{{ t('admin.mailboxes.gb') }}<input v-model.number="form.customGb" type="number" min="1" step="1" /></label>
        <div v-else-if="!isNew" class="f" style="justify-content:flex-end">
          <span class="help">{{ props.mailbox.messages != null ? t('admin.mailboxes.usedWithLetters', { used: bytes(props.mailbox.usedBytes), n: props.mailbox.messages }) : t('admin.mailboxes.used', { used: bytes(props.mailbox.usedBytes) }) }}</span>
        </div>
      </div>
      <template v-if="isNew">
        <label class="check"><input v-model="form.autoPassword" type="checkbox" />{{ t('admin.mailboxes.autoPassword') }}</label>
        <label v-if="!form.autoPassword" class="f">{{ t('admin.mailboxes.password') }}<input v-model="form.password" type="password" autocomplete="new-password" /></label>
        <div v-if="props.users.length" class="f">
          <span style="font-size:13px;color:var(--ink-2);font-weight:500">{{ t('admin.mailboxes.whoWillWork') }}</span>
          <div class="acc-list">
            <label v-for="u in props.users" :key="u.id" class="acc-item check" style="grid-template-columns:auto 1fr">
              <input v-model="form.grantUsers" type="checkbox" :value="u.id" />
              <span>{{ u.name || u.login }} <span class="hint">{{ u.login }}</span></span>
            </label>
          </div>
          <span class="help">{{ t('admin.mailboxes.rightsLater') }}</span>
        </div>
      </template>
      <template v-else>
        <div class="f">
          <span style="font-size:13px;color:var(--ink-2);font-weight:500">{{ t('admin.mailboxes.whoWorks') }}</span>
          <div class="pills" style="align-items:center">
            <span v-for="u in props.users.filter(x => access[x.id])" :key="u.id" class="pill">{{ u.login }}{{ access[u.id]!.personal ? ' · ' + t('admin.users.personal') : '' }}{{ access[u.id]!.canSend ? '' : ' · ' + t('admin.users.readShort') }}</span>
            <span v-if="!accessCount" class="muted">{{ t('admin.mailboxes.nobody') }}</span>
            <button class="link" type="button" @click="tab = 'access'">{{ t('common.edit') }}</button>
          </div>
        </div>
        <div v-if="deleted" class="danger-zone del-zone">
          <span><b style="font-weight:600">{{ t('admin.mailboxes.deletedTitle') }}</b> {{ t('admin.mailboxes.deletedText', { by: props.mailbox.deletedBy, date: purgeDate }) }}</span>
          <div class="del-actions">
            <button class="btn primary" @click="restoreMailbox"><AppIcon name="undo" />{{ t('admin.mailboxes.restore') }}</button>
            <input v-model="delConfirm" type="text" :placeholder="props.mailbox.address" :aria-label="t('admin.mailboxes.confirmAddress')" />
            <button class="btn danger" :disabled="!typedOk || delBusy" @click="deleteMailbox(true)">{{ t('admin.mailboxes.eraseNow') }}</button>
          </div>
        </div>
        <div v-if="!deleted" class="danger-zone" style="flex-direction:row;align-items:center;flex-wrap:wrap">
          <span style="flex:1;min-width:200px"><b style="font-weight:600">{{ form.active ? t('admin.mailboxes.activeTitle') : t('admin.mailboxes.offTitle') }}</b><br>
            <span class="hint">{{ form.active ? t('admin.mailboxes.activeText') : t('admin.mailboxes.offText') }}</span></span>
          <button class="btn" :class="{ danger: form.active }" @click="toggleActive">{{ form.active ? t('admin.mailboxes.disable') : t('admin.mailboxes.enable') }}</button>
        </div>
        <div v-if="!deleted" class="danger-zone del-zone">
          <span><b style="font-weight:600">{{ t('admin.mailboxes.deleteTitle') }}</b><br>
            <span class="hint">{{ props.mailbox.external ? t('admin.mailboxes.deleteHintExternal') : t('admin.mailboxes.deleteHint') }}</span></span>
          <div class="del-actions">
            <input v-model="delConfirm" type="text" :placeholder="props.mailbox.address" :aria-label="t('admin.mailboxes.confirmAddress')" />
            <button class="btn danger" :disabled="!typedOk || delBusy" @click="deleteMailbox(false)"><AppIcon name="trash" />{{ t('common.delete') }}</button>
            <button class="link danger-text" :disabled="!typedOk || delBusy" @click="deleteMailbox(true)">{{ t('admin.mailboxes.eraseImmediately') }}</button>
          </div>
        </div>
      </template>
    </div>

    <div v-else-if="tab === 'access'" class="form">
      <p class="hint" style="margin:0">{{ t('admin.mailboxes.accessHint', { address: props.mailbox.address }) }}</p>
      <div class="acc-list">
        <div v-for="u in props.users" :key="u.id" class="acc-item">
          <input type="checkbox" :checked="!!access[u.id]" :aria-label="u.login"
                 @change="toggleUser(u.id, ($event.target as HTMLInputElement).checked)" />
          <div style="min-width:0"><b style="font-weight:500">{{ u.name || u.login }}</b><span class="hint" style="display:block">{{ u.login }}{{ u.active ? '' : ' · ' + t('admin.users.mailboxOff') }}</span></div>
          <div v-if="access[u.id]" class="opts">
            <label class="check"><input v-model="access[u.id]!.canSend" type="checkbox" :disabled="access[u.id]!.personal" />{{ t('admin.users.canSend') }}</label>
            <label class="check"><input type="checkbox" :checked="access[u.id]!.personal" @change="setPersonal(u.id)" />{{ t('admin.users.personal') }}</label>
          </div>
        </div>
        <div v-if="!props.users.length" class="empty-card">{{ t('admin.mailboxes.noUsers') }}</div>
      </div>
    </div>

    <div v-else-if="tab === 'sign'" class="form">
      <p class="hint" style="margin:0">{{ t('admin.mailboxes.signHint', { name: '{name}', login: '{login}', email: '{email}' }) }}
        <template v-if="props.mailbox.users.some((u: any) => u.personal)"> {{ t('admin.mailboxes.signPersonal') }}</template></p>
      <div class="f">{{ t('admin.mailboxes.signature') }}<SignatureEditor v-model="form.signature" :placeholder="t('admin.mailboxes.signaturePlaceholder', { name: '{name}' })" />
        <span class="help">{{ t('admin.mailboxes.signatureHelp') }}</span></div>
      <label class="f">{{ t('admin.mailboxes.template') }}<textarea v-model="form.template" rows="4" :placeholder="t('admin.mailboxes.templatePlaceholder')" />
        <span class="help">{{ t('admin.mailboxes.templateHelp') }}</span></label>
    </div>

    <div v-else class="form">
      <p v-if="props.mailbox.external" class="notice">{{ t('admin.mailboxes.externalNotice') }}</p>
      <template v-else>
      <MailClientCard :address="props.mailbox.address" :name="form.name" :password="shownPass" />
      <p class="hint" style="margin:0">{{ t('admin.mailboxes.passwordHidden') }}</p>
      <div class="pass-set">
        <input v-model="ownPass" type="text" autocomplete="off" :placeholder="t('admin.mailboxes.ownPasswordPlaceholder')" />
        <button class="btn" :disabled="!!ownPass && ownPass.length < 10" @click="resetPassword"><AppIcon name="key" />{{ ownPass ? t('admin.mailboxes.setThisPassword') : t('admin.mailboxes.generatePassword') }}</button>
      </div>
      </template>
    </div>

    <p v-if="error" role="alert" style="color:var(--danger);margin:12px 0 0">{{ error }}</p>
    <template #foot>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button v-if="tab !== 'conn'" class="btn primary" :disabled="saving" @click="save">{{ saving ? t('common.saving') : isNew ? t('admin.mailboxes.createMailbox') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>

<style scoped>
.del-zone{flex-direction:column;align-items:stretch;gap:10px}
.del-actions{display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.del-actions input{flex:1;min-width:200px;height:34px;border:1px solid var(--line);border-radius:4.5px;padding:0 10px;background:var(--surface-2);color:var(--ink);font:inherit}
</style>
