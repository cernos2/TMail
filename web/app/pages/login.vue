<script setup lang="ts">
import { ref } from 'vue'
import { api } from '~/composables/api'
import { t } from '~/composables/i18n'

const login = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (!login.value.trim() || !password.value) { error.value = t('login.enterBoth'); return }
  busy.value = true
  try {
    await api('/api/auth/login', { method: 'POST', body: { login: login.value.trim(), password: password.value } })
    const next = useRoute().query.next
    // Only an address inside TMAIL (not "//other.site").
    await navigateTo(typeof next === 'string' && next.startsWith('/') && !next.startsWith('//') ? next : '/')
  } catch (e: any) {
    error.value = e.message
    password.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="login">
    <form class="login-card veil-host" novalidate @submit.prevent="submit">
      <LoadingVeil :show="busy" :text="t('login.signingIn')" />
      <BrandLogo size="lg" />
      <div>
        <h1>{{ t('login.title') }}</h1>
        <p>{{ t('login.hint') }}</p>
      </div>
      <div class="field">
        <label for="login">{{ t('common.login') }}</label>
        <input id="login" v-model="login" autocomplete="username" autocapitalize="none" spellcheck="false" autofocus />
      </div>
      <div class="field">
        <label for="password">{{ t('common.password') }}</label>
        <input id="password" v-model="password" type="password" autocomplete="current-password" />
      </div>
      <p v-if="error" class="login-err" role="alert">{{ error }}</p>
      <button class="btn primary" type="submit" :disabled="busy">{{ busy ? t('login.checking') : t('login.submit') }}</button>
    </form>
    <PoweredBy class="login-powered" />
  </main>
</template>
