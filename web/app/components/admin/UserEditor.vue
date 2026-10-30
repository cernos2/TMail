<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { adminApi } from '~/composables/admin'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { confirmDialog, showSecret } from '~/composables/dialogs'
import { LANGS, t, fmtDate } from '~/composables/i18n'

type Grant = { mailboxId: number, canSend: boolean, personal: boolean }
const props = defineProps<{ user: any | null, mailboxes: any[] }>()
const emit = defineEmits<{ close: [], saved: [] }>()

const isNew = !props.user
const tab = ref('main')
const form = reactive({
  login: props.user?.login || '',
  name: props.user?.name || '',
  isAdmin: props.user?.isAdmin || false,
  active: props.user?.active ?? true,
  autoPassword: true,
  password: '',
  lang: props.user?.lang || '',
  timezone: props.user?.timezone || '',
})
const grants = reactive<Record<number, Grant | undefined>>({})
for (const g of props.user?.mailboxes || []) grants[g.mailboxId] = { mailboxId: g.mailboxId, canSend: g.canSend, personal: g.personal }
const error = ref('')
const saving = ref(false)
const isSelf = computed(() => !!props.user && props.user.login === mail.me?.login)

// Interface language and time zone of the user; '' keeps the server default.
const serverLang = computed(() => LANGS.find(l => l.code === mail.me?.defaults?.lang)?.name || mail.me?.defaults?.lang || 'English')
const langOptions = computed(() => [{ value: '', label: t('locale.langAuto', { lang: serverLang.value }) },
  ...LANGS.map(l => ({ value: l.code, label: l.name === l.en ? l.name : `${l.name} — ${l.en}` }))])
const zones: string[] = (() => { try { return (Intl as any).supportedValuesOf('timeZone') } catch { return ['UTC'] } })()
const tzOptions = computed(() => [{ value: '', label: t('admin.users.tzAuto') }, ...zones.map(z => ({ value: z, label: z.replace(/_/g, ' ') }))])

function toggle(id: number, on: boolean) {
  if (on) grants[id] = { mailboxId: id, canSend: true, personal: false }
  else delete grants[id]
}
function setPersonal(id: number) {
  for (const g of Object.values(grants)) if (g) g.personal = g.mailboxId === id ? !g.personal : false
  const g = grants[id]
  if (g?.personal) g.canSend = true
}
// The hint with the word "Personal" in bold: one sentence, split around its placeholder.
const accessHint = computed(() => [...t('admin.users.accessHint', { personal: '\u0000' }).split('\u0000'), ''])
const grantList = () => props.mailboxes.filter(m => grants[m.id]).map(m => grants[m.id]!)

async function save() {
  error.value = ''
  if (isNew && !/^[a-z0-9][a-z0-9._-]{1,62}$/.test(form.login.trim().toLowerCase())) {
    error.value = t('admin.users.loginInvalid'); tab.value = 'main'; return
  }
  if (isNew && !form.autoPassword && form.password.length < 10) { error.value = t('admin.users.passwordShort'); tab.value = 'main'; return }
  saving.value = true
  try {
    if (isNew) {
      const res = await adminApi('/users', { method: 'POST', body: {
        login: form.login.trim().toLowerCase(), name: form.name.trim(), isAdmin: form.isAdmin,
        password: form.autoPassword ? '' : form.password, mailboxes: grantList(),
      } })
      toast(t('admin.users.created', { login: form.login }))
      if (res.password) showSecret({ title: t('admin.users.passwordFor', { login: form.login }), secret: res.password, rows: [[t('admin.users.loginAddress'), location.origin], [t('common.login'), form.login.trim().toLowerCase()]] })
      // Creation does not take a locale: set it right after, if one was chosen (the user already exists, so a failure is only reported).
      if ((form.lang || form.timezone) && res.id) await adminApi(`/users/${res.id}`, { method: 'PATCH', body: { lang: form.lang, timezone: form.timezone } }).catch((e: any) => toast(e.message, { kind: 'err' }))
    } else {
      await adminApi(`/users/${props.user.id}`, { method: 'PATCH', body: { name: form.name.trim(), isAdmin: form.isAdmin, active: form.active, lang: form.lang, timezone: form.timezone } })
      await adminApi(`/users/${props.user.id}/mailboxes`, { method: 'PUT', body: grantList() })
      toast(t('admin.users.changesSaved'))
    }
    emit('saved')
  } catch (e: any) {
    error.value = e.message
  } finally { saving.value = false }
}

async function resetPassword() {
  if (!await confirmDialog(t('admin.users.resetConfirm', { login: props.user.login }), t('admin.users.resetConfirmText'), { ok: t('admin.users.resetPassword') })) return
  try {
    const res = await adminApi(`/users/${props.user.id}/password`, { method: 'POST', body: {} })
    showSecret({ title: t('admin.users.newPasswordFor', { login: props.user.login }), secret: res.password, rows: [[t('admin.users.loginAddress'), location.origin], [t('common.login'), props.user.login]] })
    emit('saved')
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function logoutAll() {
  try {
    const res = await adminApi(`/users/${props.user.id}/logout`, { method: 'POST', body: {} })
    toast(res.closed ? t('admin.users.sessionsClosed', { n: res.closed }) : t('admin.users.noSessions'))
    emit('saved')
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function remove() {
  if (!await confirmDialog(t('admin.users.deleteConfirm', { login: props.user.login }), t('admin.users.deleteConfirmText'), { ok: t('common.delete'), danger: true })) return
  try {
    await adminApi(`/users/${props.user.id}`, { method: 'DELETE' })
    toast(t('admin.users.deleted', { login: props.user.login }))
    emit('saved')
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
</script>

<template>
  <AppModal :title="isNew ? t('admin.users.newTitle') : t('admin.users.userTitle', { login: props.user.login })" width="680px" :busy="saving ? t('common.saving') : ''" @close="emit('close')">
    <template #tabs>
      <div class="tabs" role="tablist">
        <button :class="{ on: tab === 'main' }" @click="tab = 'main'">{{ t('admin.users.tabMain') }}</button>
        <button :class="{ on: tab === 'access' }" @click="tab = 'access'">{{ t('admin.users.tabAccess', { n: grantList().length }) }}</button>
        <button v-if="!isNew" :class="{ on: tab === 'sec' }" @click="tab = 'sec'">{{ t('admin.users.tabSecurity') }}</button>
      </div>
    </template>

    <div v-if="tab === 'main'" class="form">
      <div class="row2">
        <label class="f">{{ t('common.login') }}
          <input v-model="form.login" type="text" :disabled="!isNew" autocomplete="off" autocapitalize="none" spellcheck="false" placeholder="anna.petrova" />
          <span class="help">{{ isNew ? t('admin.users.loginHelpNew') : t('admin.users.loginHelpFixed') }}</span>
        </label>
        <label class="f">{{ t('admin.users.name') }}<input v-model="form.name" type="text" :placeholder="t('admin.users.namePlaceholder')" /><span class="help">{{ t('admin.users.nameHelp', { name: '{name}' }) }}</span></label>
      </div>
      <div class="f" style="display:flex;flex-direction:column;gap:8px">
        <span style="font-size:13px;color:var(--ink-2);font-weight:500">{{ t('admin.users.role') }}</span>
        <label class="check"><input v-model="form.isAdmin" type="radio" :value="false" :disabled="isSelf" />
          <span><b style="font-weight:500">{{ t('admin.users.roleUser') }}</b> — {{ t('admin.users.roleUserDesc') }}</span></label>
        <label class="check"><input v-model="form.isAdmin" type="radio" :value="true" :disabled="isSelf" />
          <span><b style="font-weight:500">{{ t('admin.users.roleAdmin') }}</b> — {{ t('admin.users.roleAdminDesc') }}</span></label>
        <span v-if="isSelf" class="help">{{ t('admin.users.ownRoleHelp') }}</span>
      </div>
      <div class="row2">
        <label class="f">{{ t('locale.language') }}
          <AppSelect v-model="form.lang" :options="langOptions" :aria-label="t('locale.language')" searchable /></label>
        <label class="f">{{ t('locale.timezone') }}
          <AppSelect v-model="form.timezone" :options="tzOptions" :aria-label="t('locale.timezone')" searchable /></label>
      </div>
      <template v-if="isNew">
        <label class="check"><input v-model="form.autoPassword" type="checkbox" />{{ t('admin.users.autoPassword') }}</label>
        <label v-if="!form.autoPassword" class="f">{{ t('common.password') }}<input v-model="form.password" type="password" autocomplete="new-password" /><span class="help">{{ t('admin.users.passwordMin') }}</span></label>
      </template>
      <label v-else class="check"><input v-model="form.active" type="checkbox" :disabled="isSelf" />{{ t('admin.users.accountActive') }}
        <span class="help">{{ t('admin.users.accountActiveHelp') }}</span></label>
    </div>

    <div v-else-if="tab === 'access'" class="form">
      <p class="hint" style="margin:0">{{ accessHint[0] }}<b>{{ t('admin.users.personalTitle') }}</b>{{ accessHint[1] }}</p>
      <div class="acc-list">
        <div v-for="m in props.mailboxes" :key="m.id" class="acc-item">
          <input type="checkbox" :checked="!!grants[m.id]" :aria-label="m.address"
                 @change="toggle(m.id, ($event.target as HTMLInputElement).checked)" />
          <div style="min-width:0"><b style="font-weight:500">{{ m.address }}</b><span class="hint" style="display:block">{{ m.name || '—' }}{{ m.active ? '' : ' · ' + t('admin.users.mailboxOff') }}</span></div>
          <div v-if="grants[m.id]" class="opts">
            <label class="check"><input type="checkbox" v-model="grants[m.id]!.canSend" :disabled="grants[m.id]!.personal" />{{ t('admin.users.canSend') }}</label>
            <label class="check"><input type="checkbox" :checked="grants[m.id]!.personal" @change="setPersonal(m.id)" />{{ t('admin.users.personal') }}</label>
          </div>
        </div>
        <div v-if="!props.mailboxes.length" class="empty-card">{{ t('admin.users.noMailboxes') }}</div>
      </div>
    </div>

    <div v-else class="form">
      <div class="card"><div class="kv">
        <div>{{ t('admin.users.openSessions') }}</div><div>{{ props.user.sessions }}</div>
        <div>{{ t('admin.users.createdAt') }}</div><div>{{ fmtDate(props.user.createdAt, { day: '2-digit', month: '2-digit', year: 'numeric' }) }}</div>
      </div></div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        <button class="btn" @click="resetPassword"><AppIcon name="key" />{{ t('admin.users.resetPassword') }}</button>
        <button class="btn" :disabled="!props.user.sessions" @click="logoutAll"><AppIcon name="logout" />{{ t('admin.users.logoutAll') }}</button>
      </div>
      <div v-if="!isSelf" class="danger-zone">
        <b style="font-weight:600">{{ t('admin.users.deletion') }}</b>
        <span class="hint">{{ t('admin.users.deletionHint') }}</span>
        <div><button class="btn danger" @click="remove"><AppIcon name="trash" />{{ t('admin.users.deleteUser') }}</button></div>
      </div>
    </div>

    <p v-if="error" class="err" role="alert" style="color:var(--danger);margin:12px 0 0">{{ error }}</p>
    <template #foot>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="btn primary" :disabled="saving" @click="save">{{ saving ? t('common.saving') : isNew ? t('admin.users.createUser') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>
