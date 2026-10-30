<script setup lang="ts">
import { ref, watch, computed, provide, onMounted, onBeforeUnmount } from 'vue'
import { mail, logout, closeMessage, loadMe, loadFolders, loadMessages, unreadInbox, closeCompose, act, watchCompose } from '~/composables/mail'
import { connectEvents, disconnectEvents } from '~/composables/realtime'
import { cal } from '~/composables/calendar'
import { crm } from '~/composables/crm'
import { loadAIActions } from '~/composables/ai'
import { folders } from '~/composables/folders'
import { labels } from '~/composables/labels'
import { initNotify, notifyState, setUnreadBadge } from '~/composables/notify'
import { applyAppearance, savedAppearance, type Theme } from '~/composables/schemes'
import { api } from '~/composables/api'
import { applyQuery, syncUrl } from '~/composables/urlstate'
import { t, i18n } from '~/composables/i18n'

watchCompose()
const ready = ref(false)
const fatal = ref('')
const lookOpen = ref(false)
try { document.documentElement.dataset.unread = localStorage.getItem('tmail.unread') || 'bg' } catch {}
// Contacts take the place of the letter list; the calendar is shown when no letter is open.
const crmMode = computed(() => crm.open && !mail.current && !mail.currentLoading)
const calShown = computed(() => !crm.open && !mail.current && !mail.currentLoading)
function showMail() { crm.open = false; mail.reading = false }
function showCalendar() { closeMessage(); crm.open = false; mail.reading = true }
function showCrm() { closeMessage(); crm.open = true; mail.reading = true }
// Widths of the folder column and the letter list, dragged by the handles on their right edges.
// Kept on the server for the user (the same on every computer), with a local copy for a fast start.
const listW = ref(410)
const sideW = ref(248)
const clampList = (w: number) => Math.round(Math.min(900, Math.max(300, w)))
const clampSide = (w: number) => Math.round(Math.min(480, Math.max(190, w)))
try {
  const v = Number(localStorage.getItem('tmail.listW')); if (v) listW.value = clampList(v)
  const sv = Number(localStorage.getItem('tmail.sideW')); if (sv) sideW.value = clampSide(sv)
} catch {}
let widthsTimer: ReturnType<typeof setTimeout> | undefined
function saveWidths() {
  try { localStorage.setItem('tmail.listW', String(listW.value)); localStorage.setItem('tmail.sideW', String(sideW.value)) } catch {}
  clearTimeout(widthsTimer) // to the server once the dragging stops
  widthsTimer = setTimeout(() => api('/api/settings', { method: 'PUT', body: { listW: listW.value, sideW: sideW.value } }).catch(() => {}), 700)
}
function setListW(w: number) { listW.value = clampList(w); saveWidths() }
function setSideW(w: number) { sideW.value = clampSide(w); saveWidths() }
provide('setListW', setListW)
provide('listW', listW)
provide('setSideW', setSideW)
provide('sideW', sideW)
const lookTab = ref<string | undefined>(undefined)
// Other parts of the app open settings on a given tab through mail.settingsTab.
watch(() => mail.settingsTab, tab => { if (tab) { lookTab.value = tab; lookOpen.value = true; mail.settingsTab = '' } })
watch(lookOpen, o => { if (!o) lookTab.value = undefined })
const look = ref(savedAppearance())

let urlSynced = false
async function start() {
  fatal.value = ''
  try {
    await loadMe()
  } catch (e: any) {
    if (e.status !== 401) fatal.value = e.message
    return
  }
  ready.value = true
  api<Record<string, any>>('/api/settings').then(s => {
    mail.defaultFrom = typeof s.defaultFrom === 'string' ? s.defaultFrom : ''
    if (Number(s.listW)) listW.value = clampList(Number(s.listW))
    if (Number(s.sideW)) sideW.value = clampSide(Number(s.sideW))
    if (s.scheme || s.theme) {
      look.value = { scheme: s.scheme || look.value.scheme, theme: s.theme || look.value.theme }
      applyAppearance(look.value.scheme, look.value.theme)
    }
    initNotify(s)
  }).catch(() => initNotify())
  loadFolders().catch(() => {})
  // The address says what to show (a link from a colleague or another browser); then it follows the screen.
  const router = useRouter()
  await applyQuery(router.currentRoute.value.query, true)
  if (!urlSynced) { urlSynced = true; syncUrl(router) }
  connectEvents()
}

function saveLook(patch: Record<string, any>) {
  api('/api/settings', { method: 'PUT', body: patch }).catch(() => {})
}
function setScheme(id: string) { look.value.scheme = id; applyAppearance(id, look.value.theme); saveLook({ scheme: id }) }
function setTheme(th: Theme) { look.value.theme = th; applyAppearance(look.value.scheme, th); saveLook({ theme: th }) }
function setNotify(kind: 'notify' | 'notifyFiled' | 'notifyCalendar', on: boolean) {
  if (kind === 'notify') notifyState.enabled = on
  else if (kind === 'notifyFiled') notifyState.filed = on
  else notifyState.calendar = on
  saveLook({ [kind]: on })
}

// The tab title follows the unread count and the interface language.
watch(() => [unreadInbox.value, i18n.version] as const, ([n]) => setUnreadBadge(n))

function onKey(e: KeyboardEvent) {
  const el = e.target as HTMLElement
  if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable)) return
  if (e.key === 'Escape') {
    if (lookOpen.value) lookOpen.value = false
    else if (mail.compose && mail.composeModal) closeCompose()
    else if (mail.sideOpen) mail.sideOpen = false
    else if (mail.reading) mail.reading = false
  }
  if (e.key === 'Delete' && mail.current) act('delete', [mail.current])
}

onMounted(() => { start(); loadAIActions(); window.addEventListener('keydown', onKey) })
onBeforeUnmount(() => { disconnectEvents(); window.removeEventListener('keydown', onKey) })
</script>

<template>
  <div class="shell">
    <div v-if="fatal" class="login">
      <div class="login-card"><h1>{{ t('app.unavailable') }}</h1><p>{{ fatal }}</p><button class="btn primary" @click="start">{{ t('common.retry') }}</button></div>
    </div>
    <div v-else-if="ready" class="app" :class="{ reading: mail.reading, 'side-open': mail.sideOpen, 'crm-mode': crmMode }" :style="{ '--list-w': listW + 'px', '--side-w': sideW + 'px' }">
      <nav class="rail" :aria-label="t('app.sections')">
        <button class="rail-btn" :class="{ active: !crm.open || mail.current }" :aria-label="t('app.mail')" @click="showMail"><AppIcon name="mail" /><span class="tip">{{ t('app.mail') }}</span></button>
        <button class="rail-btn" :class="{ active: calShown }" :aria-label="t('side.calendar')" @click="showCalendar"><AppIcon name="cal" /><span class="tip">{{ t('side.calendar') }}</span></button>
        <button class="rail-btn" :class="{ active: crmMode }" :aria-label="t('side.contacts')" @click="showCrm"><AppIcon name="users" /><span class="tip">{{ t('side.contacts') }}</span></button>
        <div class="spacer"></div>
        <NuxtLink v-if="mail.me?.isAdmin" to="/admin" class="rail-btn" :aria-label="t('side.admin')"><AppIcon name="settings" /><span class="tip">{{ t('side.admin') }}</span></NuxtLink>
        <button class="rail-btn" :aria-label="t('app.settingsLook')" @click="lookOpen = true"><AppIcon name="palette" /><span class="tip">{{ t('app.settingsLook') }}</span></button>
        <button class="rail-btn rail-me" :aria-label="t('app.profile', { name: mail.me?.name || mail.me?.login })" @click="mail.settingsTab = 'profile'">
          <AppAvatar :login="mail.me?.login" :name="mail.me?.name || mail.me?.login" style="width:34px;height:34px;font-size:12px" />
          <span class="tip">{{ mail.me?.name || mail.me?.login }} · {{ mail.me?.login }}</span></button>
        <button class="rail-btn" :aria-label="t('side.logout')" @click="logout"><AppIcon name="logout" /><span class="tip">{{ t('side.logout') }}</span></button>
      </nav>
      <SideBar :scheme="look.scheme" @look="lookOpen = true; mail.sideOpen = false" />
      <div class="scrim-side" @click="mail.sideOpen = false"></div>
      <MessageList />
      <MessageReader />
      <AttachmentPreview />
      <EventModal v-if="cal.editing" />
      <FolderModal v-if="folders.edit" />
      <LabelModal v-if="labels.edit" />
    </div>

    <ComposeWindow v-if="mail.compose && mail.composeModal" />
    <ReminderStack />

    <LookModal v-if="lookOpen" :scheme="look.scheme" :theme="look.theme" :tab="lookTab"
               @close="lookOpen = false" @scheme="setScheme" @theme="setTheme" @notify="setNotify" />
  </div>
</template>
