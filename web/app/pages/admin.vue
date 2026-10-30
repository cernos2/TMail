<script setup lang="ts">
import { ref, computed, watch, onMounted, provide } from 'vue'
import { mail, loadMe, logout } from '~/composables/mail'
import { initials, avatarColor } from '~/composables/format'
import { t } from '~/composables/i18n'

const route = useRoute()
const ready = ref(false)
const sideOpen = ref(false)
provide('openAdminMenu', () => { sideOpen.value = true })
watch(() => route.path, () => { sideOpen.value = false })

const NAV = computed(() => [
  { to: '/admin', icon: 'home', label: t('admin.navOverview') },
  { to: '/admin/users', icon: 'users', label: t('admin.navUsers') },
  { to: '/admin/mailboxes', icon: 'mail', label: t('admin.navMailboxes') },
  { to: '/admin/aliases', icon: 'at', label: t('admin.navAliases') },
  { to: '/admin/domains', icon: 'globe', label: t('admin.navDomains') },
  { to: '/admin/ai', icon: 'sparkle', label: t('admin.navAi') },
  { to: '/admin/collector', icon: 'download', label: t('admin.navCollector') },
  { to: '/admin/senders', icon: 'shieldX', label: t('admin.navSenders') },
  { to: '/admin/antivirus', icon: 'shield', label: t('admin.navAntivirus') },
  { to: '/admin/server', icon: 'server', label: t('admin.navServer') },
  { to: '/admin/audit', icon: 'list', label: t('admin.navAudit') },
])
const isActive = (to: string) => to === '/admin' ? route.path === '/admin' || route.path === '/admin/' : route.path.startsWith(to)

onMounted(async () => {
  if (!mail.me) { try { await loadMe() } catch { return } }
  if (!mail.me?.isAdmin) { await navigateTo('/'); return }
  ready.value = true
})
</script>

<template>
  <div class="shell">
    <div v-if="ready" class="admin" :class="{ 'side-open': sideOpen }">
      <nav class="rail" :aria-label="t('admin.sections')">
        <NuxtLink to="/" class="rail-btn" :aria-label="t('admin.mail')"><AppIcon name="mail" /><span class="tip">{{ t('admin.mail') }}</span></NuxtLink>
        <span class="rail-btn active" aria-current="page"><AppIcon name="settings" /><span class="tip">{{ t('admin.title') }}</span></span>
        <div class="spacer"></div>
      </nav>
      <aside class="adm-nav" :aria-label="t('admin.title')">
        <div class="side-head"><BrandLogo /></div>
        <div class="sub-title">{{ t('admin.title') }}</div>
        <div class="side-scroll">
          <nav class="nav">
            <NuxtLink v-for="n in NAV" :key="n.to" :to="n.to" class="nav-item" :class="{ active: isActive(n.to) }" style="text-decoration:none">
              <AppIcon :name="n.icon" /><span class="label">{{ n.label }}</span>
            </NuxtLink>
          </nav>
        </div>
        <div class="side-foot">
          <NuxtLink to="/" class="look-btn" style="text-decoration:none"><AppIcon name="back" /><span>{{ t('admin.backToMail') }}</span></NuxtLink>
          <div class="user-row">
            <div class="avatar" :style="{ background: avatarColor(mail.me?.login || ''), width: '32px', height: '32px', fontSize: '12px' }">{{ initials(mail.me?.name || mail.me?.login || '') }}</div>
            <div class="who"><b>{{ mail.me?.name || mail.me?.login }}</b><span>{{ t('admin.roleAdmin') }}</span></div>
            <button class="icon-btn" :title="t('admin.logout')" :aria-label="t('admin.logout')" @click="logout"><AppIcon name="logout" /></button>
          </div>
        </div>
      </aside>
      <div class="scrim-side" @click="sideOpen = false"></div>
      <main class="adm-main"><NuxtPage /></main>
    </div>
  </div>
</template>
