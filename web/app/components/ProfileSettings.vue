<script setup lang="ts">
// Own photo: shown to colleagues in TMAIL next to letters from the user's personal mailbox,
// in the sidebar and in the admin list.
import { ref, computed, onMounted } from 'vue'
import { api } from '~/composables/api'
import { mail } from '~/composables/mail'
import { avatars, avatarChanged } from '~/composables/avatar'
import { toast } from '~/composables/toast'
import { t } from '~/composables/i18n'

const file = ref<HTMLInputElement | null>(null)
const busy = ref(false)
const hasPhoto = ref(false)
const personal = computed(() => mail.me?.accounts.find(a => a.personal)?.address || '')

async function check() {
  const r = await fetch(`/api/user-photo?login=${encodeURIComponent(mail.me?.login || '')}&v=${avatars.version}`, { credentials: 'same-origin' })
  hasPhoto.value = r.ok
}

async function upload(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (file.value) file.value.value = ''
  if (!f) return
  if (f.size > 5 * 1024 * 1024) { toast(t('profile.photoTooBig'), { kind: 'err' }); return }
  busy.value = true
  try {
    const fd = new FormData()
    fd.append('photo', f)
    await api('/api/me/avatar', { method: 'POST', body: fd })
    avatarChanged()
    hasPhoto.value = true
    toast(t('profile.photoUpdated'))
  } catch (err: any) {
    toast(err.message, { kind: 'err' })
  } finally { busy.value = false }
}

async function remove() {
  busy.value = true
  try {
    await api('/api/me/avatar', { method: 'DELETE' })
    avatarChanged()
    hasPhoto.value = false
    toast(t('profile.photoDeleted'))
  } catch (err: any) {
    toast(err.message, { kind: 'err' })
  } finally { busy.value = false }
}

onMounted(check)
</script>

<template>
  <div class="profile veil-host">
    <LoadingVeil :show="busy" :text="t('profile.savingPhoto')" />
    <AppAvatar :login="mail.me?.login" :name="mail.me?.name || mail.me?.login" class="profile-ava" />
    <div class="profile-info">
      <b>{{ mail.me?.name || mail.me?.login }}</b>
      <span class="hint">{{ t('profile.login', { login: mail.me?.login }) }}</span>
      <div class="profile-btns">
        <button class="btn primary" :disabled="busy" @click="file?.click()"><AppIcon name="image" />{{ hasPhoto ? t('profile.replacePhoto') : t('profile.uploadPhoto') }}</button>
        <button v-if="hasPhoto" class="btn" :disabled="busy" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
        <input ref="file" type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden @change="upload" />
      </div>
      <span class="hint">{{ t('profile.photoHint') }}
        <template v-if="personal">{{ t('profile.photoColleagues', { address: personal }) }}</template></span>
    </div>
    <div class="profile-locale">
      <h3>{{ t('locale.title') }}</h3>
      <LocaleSettings />
    </div>
    <PoweredBy class="profile-powered" />
  </div>
</template>

<style scoped>
.profile{display:flex;gap:20px;align-items:flex-start;flex-wrap:wrap}
.profile-ava{width:96px;height:96px;font-size:32px}
.profile-info{display:flex;flex-direction:column;gap:8px;flex:1;min-width:240px}
.profile-info b{font-size:16px;font-weight:600}
.profile-btns{display:flex;gap:8px;flex-wrap:wrap;margin-top:4px}
.profile-locale{flex-basis:100%;border-top:1px solid var(--line);padding-top:16px;display:flex;flex-direction:column;gap:12px}
.profile-powered{flex-basis:100%;text-align:left}
.profile-locale h3{margin:0;font-size:15px;font-weight:600}
</style>
