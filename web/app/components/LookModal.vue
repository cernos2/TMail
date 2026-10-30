<script setup lang="ts">
// Personal settings available to every user: look, signatures/templates, notifications, photo.
import { ref, computed } from 'vue'
import { mail, setImagePrefs } from '~/composables/mail'
import { BACKGROUNDS, applyBackground } from '~/composables/backgrounds'
import { api } from '~/composables/api'
import { toast } from '~/composables/toast'
import { SCHEMES, type Theme } from '~/composables/schemes'
import { notifyState, requestNotifyPermission, testNotification } from '~/composables/notify'
import { t } from '~/composables/i18n'

const props = defineProps<{ scheme: string, theme: Theme, tab?: string }>()
const emit = defineEmits<{ close: [], scheme: [string], theme: [Theme], notify: ['notify' | 'notifyFiled' | 'notifyCalendar', boolean] }>()
const THEMES = computed<{ id: Theme, name: string }[]>(() => [{ id: 'system', name: t('settings.themeSystem') }, { id: 'light', name: t('settings.themeLight') }, { id: 'dark', name: t('settings.themeDark') }])
const tab = ref(props.tab || 'look')
// How unread letters stand out in the list (kept in this browser).
const UNREAD = computed(() => [{ id: 'bg', name: t('settings.unreadBg') }, { id: 'bar', name: t('settings.unreadBar') }, { id: 'bold', name: t('settings.unreadBold') }])
const unreadStyle = ref(document.documentElement.dataset.unread || 'bg')
function setUnreadStyle(v: string) {
  unreadStyle.value = v
  document.documentElement.dataset.unread = v
  try { localStorage.setItem('tmail.unread', v) } catch {}
}

// ---------- background ----------
const bgUploading = ref(false)
const bgFile = ref<HTMLInputElement | null>(null)
const bgNow = () => mail.me?.background || ''
async function setBg(id: string) {
  try {
    const r = await api<{ background: string, backgroundVersion: number }>('/api/me/background', { method: 'POST', body: { background: id } })
    Object.assign(mail.me!, r); applyBackground(r.background, r.backgroundVersion)
  } catch (e: any) { toast(e.message, { kind: 'err' }) }
}
async function uploadBg(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  bgUploading.value = true
  try {
    const fd = new FormData(); fd.append('photo', f)
    const r = await fetch('/api/me/background/upload', { method: 'POST', body: fd, credentials: 'same-origin', headers: { 'X-ZM': '1' } })
    const j = await r.json()
    if (!r.ok) throw new Error(j.error || t('settings.uploadFailed'))
    Object.assign(mail.me!, j); applyBackground(j.background, j.backgroundVersion)
    toast(t('settings.bgSet'))
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { bgUploading.value = false; if (bgFile.value) bgFile.value.value = '' }
}
</script>

<template>
  <AppModal :title="t('common.settings')" width="1120px" :busy="bgUploading ? t('settings.uploadingBg') : ''" @close="emit('close')">
    <template #tabs>
      <div class="tabs" role="tablist">
        <button :class="{ on: tab === 'look' }" role="tab" @click="tab = 'look'">{{ t('settings.tabLook') }}</button>
        <button :class="{ on: tab === 'sign' }" role="tab" @click="tab = 'sign'">{{ t('settings.tabSign') }}</button>
        <button :class="{ on: tab === 'notify' }" role="tab" @click="tab = 'notify'">{{ t('settings.tabNotify') }}</button>
        <button :class="{ on: tab === 'templates' }" role="tab" @click="tab = 'templates'">{{ t('settings.tabTemplates') }}</button>
        <button :class="{ on: tab === 'autoreply' }" role="tab" @click="tab = 'autoreply'">{{ t('settings.tabAutoreply') }}</button>
        <button :class="{ on: tab === 'clients' }" role="tab" @click="tab = 'clients'">{{ t('settings.tabClients') }}</button>
        <button :class="{ on: tab === 'mcp' }" role="tab" @click="tab = 'mcp'">{{ t('settings.tabMcp') }}</button>
        <button :class="{ on: tab === 'profile' }" role="tab" @click="tab = 'profile'">{{ t('settings.tabProfile') }}</button>
      </div>
    </template>

    <div v-if="tab === 'look'" style="display:flex;flex-direction:column;gap:16px">
      <div style="display:flex;flex-direction:column;gap:10px">
        <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">
          <b style="font-weight:600">{{ t('settings.unreadTitle') }}</b>
          <span class="seg" role="group" :aria-label="t('settings.unreadAria')">
            <button v-for="u in UNREAD" :key="u.id" :class="{ on: unreadStyle === u.id }" @click="setUnreadStyle(u.id)">{{ u.name }}</button>
          </span>
        </div>
        <div class="unread-demo" aria-hidden="true">
          <div class="row unread"><div class="row-main"><div class="row-from"><b>{{ t('settings.demoSender1') }}</b></div><div class="row-subj">{{ t('settings.demoSubject1') }}</div></div><div class="row-meta"><span class="row-time">12:40</span></div></div>
          <div class="row"><div class="row-main"><div class="row-from"><b>{{ t('settings.demoSender2') }}</b></div><div class="row-subj">{{ t('settings.demoSubject2') }}</div></div><div class="row-meta"><span class="row-time">11:02</span></div></div>
        </div>
      </div>
      <div style="display:flex;flex-direction:column;gap:8px">
        <b style="font-weight:600">{{ t('settings.background') }}</b>
        <div class="bg-grid" role="radiogroup" :aria-label="t('settings.background')">
          <button type="button" class="bg-tile none" :class="{ on: !bgNow() }" role="radio" :aria-checked="!bgNow()" @click="setBg('')"><span>{{ t('settings.noBackground') }}</span></button>
          <button v-for="b in BACKGROUNDS" :key="b.id" type="button" class="bg-tile" :class="{ on: bgNow() === b.id }" role="radio" :aria-checked="bgNow() === b.id" :title="b.name" :style="{ background: b.css, backgroundSize: 'cover', backgroundPosition: 'center' }" @click="setBg(b.id)"><span>{{ b.name }}</span></button>
          <button v-if="(mail.me?.backgroundVersion || 0) > 0" type="button" class="bg-tile" :class="{ on: bgNow() === 'custom' }" role="radio" :aria-checked="bgNow() === 'custom'" :title="t('settings.customImage')" :style="{ backgroundImage: `url(/api/me/background/image?v=${mail.me?.backgroundVersion})`, backgroundSize: 'cover', backgroundPosition: 'center' }" @click="setBg('custom')"><span>{{ t('settings.custom') }}</span></button>
          <label class="bg-tile upload" :class="{ busy: bgUploading }"><AppIcon name="image" /><span>{{ bgUploading ? t('settings.uploading') : t('settings.uploadOwn') }}</span>
            <input ref="bgFile" type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden @change="uploadBg" /></label>
        </div>
        <span class="hint">{{ t('settings.bgHint') }}</span>
      </div>
      <div style="display:flex;flex-direction:column;gap:8px">
        <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">
          <b style="font-weight:600">{{ t('settings.imagesTitle') }}</b>
          <span class="seg" role="group" :aria-label="t('settings.imagesAria')">
            <button :class="{ on: (mail.me?.imagePolicy || 'ask') === 'ask' }" @click="setImagePrefs({ policy: 'ask' })">{{ t('settings.imagesAsk') }}</button>
            <button :class="{ on: mail.me?.imagePolicy === 'always' }" @click="setImagePrefs({ policy: 'always' })">{{ t('settings.imagesAlways') }}</button>
          </span>
        </div>
        <span class="hint">{{ t('settings.imagesHint') }}</span>
        <div v-if="mail.me?.imageSenders?.length" class="img-senders">
          <span class="hint">{{ t('settings.imagesAlwaysFrom') }}</span>
          <span v-for="x in mail.me.imageSenders" :key="x" class="chip">{{ x }}<button type="button" class="x" :aria-label="t('settings.removeSender', { sender: x })" @click="setImagePrefs({ remove: x })"><AppIcon name="close" /></button></span>
        </div>
      </div>
      <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">
        <b style="font-weight:600">{{ t('settings.theme') }}</b>
        <span class="seg" role="group" :aria-label="t('settings.theme')">
          <button v-for="th in THEMES" :key="th.id" :class="{ on: props.theme === th.id }" @click="emit('theme', th.id)">{{ th.name }}</button>
        </span>
      </div>
      <div class="schemes">
        <button v-for="c in SCHEMES" :key="c.id" class="scheme" :class="{ on: props.scheme === c.id }" :aria-pressed="props.scheme === c.id" @click="emit('scheme', c.id)">
          <div class="scheme-name"><span class="dot" :style="{ background: c.l.accent }"></span>{{ c.name }}</div>
          <div class="mini">
            <div v-for="(v, idx) in [c.l, c.d]" :key="idx">
              <div class="mini-win" :style="{ background: idx ? '#131A21' : '#FFFFFF' }">
                <div class="mini-side" :style="{ background: v.s2 }"><i class="b" :style="{ background: v.accent }"></i><i :style="{ background: v.soft }"></i><i :style="{ background: v.s3 }"></i></div>
                <div class="mini-main"><i :style="{ background: idx ? '#E4EAF0' : '#111A22', opacity: .5, width: '70%' }"></i><i :style="{ background: v.s3 }"></i><i :style="{ background: v.soft, width: '60%' }"></i></div>
              </div>
              <div class="mini-cap">{{ idx ? t('settings.previewDark') : t('settings.previewLight') }}</div>
            </div>
          </div>
        </button>
      </div>
    </div>

    <SignatureSettings v-else-if="tab === 'sign'" />

    <ProfileSettings v-else-if="tab === 'profile'" />
    <McpSettings v-else-if="tab === 'mcp'" />

    <div v-else-if="tab === 'clients'" style="display:flex;flex-direction:column;gap:12px">
      <p class="hint" style="margin:0">{{ t('settings.clientsHint') }}</p>
      <MailClientCard v-for="a in mail.me?.accounts || []" :key="a.address" :address="a.address" :name="a.name" />
    </div>

    <TemplateSettings v-else-if="tab === 'templates'" />

    <AutoReplySettings v-else-if="tab === 'autoreply'" />

    <div v-else style="display:flex;flex-direction:column;gap:12px">
      <div class="ntf-row">
        <div><b>{{ t('settings.notifyNew') }}</b><span class="hint">{{ t('settings.notifyNewHint') }}</span></div>
        <span class="seg" role="group" :aria-label="t('settings.notifyNewAria')">
          <button :class="{ on: notifyState.enabled }" @click="emit('notify', 'notify', true)">{{ t('settings.show') }}</button>
          <button :class="{ on: !notifyState.enabled }" @click="emit('notify', 'notify', false)">{{ t('settings.dontShow') }}</button>
        </span>
      </div>
      <div class="ntf-row">
        <div><b>{{ t('settings.notifyFiled') }}</b><span class="hint">{{ t('settings.notifyFiledHint') }}</span></div>
        <span class="seg" role="group" :aria-label="t('settings.notifyFiledAria')">
          <button :class="{ on: notifyState.filed }" @click="emit('notify', 'notifyFiled', true)">{{ t('settings.show') }}</button>
          <button :class="{ on: !notifyState.filed }" @click="emit('notify', 'notifyFiled', false)">{{ t('settings.dontShow') }}</button>
        </span>
      </div>
      <div class="ntf-row">
        <div><b>{{ t('settings.notifyCalendar') }}</b><span class="hint">{{ t('settings.notifyCalendarHint') }}</span></div>
        <span class="seg" role="group" :aria-label="t('settings.notifyCalendarAria')">
          <button :class="{ on: notifyState.calendar }" @click="emit('notify', 'notifyCalendar', true)">{{ t('settings.show') }}</button>
          <button :class="{ on: !notifyState.calendar }" @click="emit('notify', 'notifyCalendar', false)">{{ t('settings.dontShow') }}</button>
        </span>
      </div>
      <div><button class="btn sm" @click="testNotification"><AppIcon name="bell" />{{ t('settings.testNotify') }}</button></div>
      <template v-if="notifyState.enabled || notifyState.calendar || notifyState.filed">
        <span v-if="!notifyState.supported" class="hint">{{ t('settings.notifyUnsupported') }}</span>
        <span v-else-if="notifyState.permission === 'granted'" class="badge ok" style="align-self:flex-start"><AppIcon name="check" />{{ t('settings.notifyGranted') }}</span>
        <div v-else-if="notifyState.permission === 'default'"><button class="btn primary" @click="requestNotifyPermission">{{ t('settings.notifyAllow') }}</button></div>
        <span v-else class="hint">{{ t('settings.notifyDenied') }}</span>
      </template>
      <p class="hint" style="margin:0">{{ t('settings.notifyFooter') }}</p>
    </div>
  </AppModal>
</template>

<style scoped>
/* All tabs visible: they wrap onto a second line instead of hiding behind the edge. */
.tabs{flex-wrap:wrap;overflow:visible}
.img-senders{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
.ntf-row{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;padding:10px 0;border-bottom:1px solid var(--line)}
.ntf-row > div{display:flex;flex-direction:column;gap:2px;min-width:0;flex:1}
.ntf-row .hint{font-size:12.5px}
.bg-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(112px,1fr));gap:8px}
.bg-tile{position:relative;height:68px;border-radius:5px;border:1px solid var(--line);cursor:pointer;overflow:hidden;padding:0;background:var(--surface-2);display:flex;align-items:flex-end;color:var(--ink)}
.bg-tile span{width:100%;font-size:11.5px;padding:3px 6px;background:color-mix(in srgb,var(--surface) 80%,transparent);text-align:left}
.bg-tile.on{outline:2px solid var(--accent);outline-offset:2px}
.bg-tile.none,.bg-tile.upload{align-items:center;justify-content:center;flex-direction:column;gap:4px;border-style:dashed}
.bg-tile.none span,.bg-tile.upload span{background:none;text-align:center}
.bg-tile.upload svg{width:18px;height:18px;color:var(--ink-3)}
.bg-tile.busy{opacity:.6;pointer-events:none}
.img-senders .chip{display:inline-flex;align-items:center;gap:4px}
.img-senders .x{display:grid;place-items:center;width:18px;height:18px;border:0;background:none;color:var(--ink-3);cursor:pointer}
.img-senders .x svg{width:12px;height:12px}
</style>
