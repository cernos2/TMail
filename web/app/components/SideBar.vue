<script setup lang="ts">
import { computed, inject, type Ref } from 'vue'
import { mail, setView, unseenFor, totalFor, fmtCount, accountColor, startCompose, logout, closeMessage, ROLE_NAMES, ROLE_ICONS } from '~/composables/mail'
import { SCHEMES } from '~/composables/schemes'
import { folders, newFolder, editFolder, ruleOf, loadRules } from '~/composables/folders'
import { crm } from '~/composables/crm'
import { labels, loadLabels, LABEL_COLORS } from '~/composables/labels'
import { t, num } from '~/composables/i18n'
import { onMounted } from 'vue'
import { watch } from 'vue'

const emit = defineEmits<{ look: [] }>()
const props = defineProps<{ scheme: string }>()

const ROLES = ['inbox', 'flagged', 'drafts', 'sent', 'archive', 'junk', 'trash']
const isActive = (role: string) => !mail.view.folder && !mail.view.label && mail.view.role === role
const customFolders = computed(() =>
  mail.view.account ? (mail.folders[mail.view.account] || []).filter(f => !f.role) : [])
const sc = computed(() => SCHEMES.find(s => s.id === props.scheme) ?? SCHEMES[0])

// The calendar lives in the reading pane: closing the letter shows it.
function openCalendar() { closeMessage(); crm.open = false; mail.reading = true; mail.sideOpen = false }
function openCrm() { closeMessage(); crm.open = true; mail.reading = true; mail.sideOpen = false }
const calendarShown = computed(() => !mail.current && !mail.currentLoading && !crm.open)
const crmShown = computed(() => !mail.current && !mail.currentLoading && crm.open)
watch(() => mail.view.account, a => { if (a && !folders.rules[a]) loadRules(a) }, { immediate: true })
onMounted(loadLabels)
const newFolderFor = () => newFolder(mail.view.account || mail.me?.accounts[0]?.address || '')
function pickRole(role: string) { setView({ account: mail.view.account, role }) }
function pickAccount(addr: string | null) {
  setView({ account: mail.view.account === addr && !mail.view.folder && mail.view.role === 'inbox' ? null : addr, role: 'inbox' })
}
// Folder rows are narrow: 65 809 → «65.8K», so the folder name stays readable (exact numbers in the tooltip).
function shortCount(n: number) {
  if (n < 10000) return fmtCount(n)
  return t('side.thousands', { n: n < 100000 ? num(n / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) : num(Math.round(n / 1000)) })
}
// Width of this column: dragged by the handle on its right edge (kept per user on the server).
const setSideW = inject<(w: number) => void>('setSideW')
const sideW = inject<Ref<number>>('sideW')
function startResize(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  const x0 = e.clientX, w0 = sideW?.value || 248
  el.setPointerCapture(e.pointerId)
  document.body.classList.add('resizing')
  const move = (ev: PointerEvent) => setSideW?.(w0 + ev.clientX - x0)
  const up = () => { el.removeEventListener('pointermove', move); el.removeEventListener('pointerup', up); document.body.classList.remove('resizing') }
  el.addEventListener('pointermove', move)
  el.addEventListener('pointerup', up)
  e.preventDefault()
}
</script>

<template>
  <aside class="side" :aria-label="t('side.aria')">
    <div class="col-resize" role="separator" aria-orientation="vertical" :aria-label="t('side.widthAria')" :title="t('side.widthHint')"
         @pointerdown="startResize" @dblclick="setSideW?.(248)"></div>
    <div class="side-head"><BrandLogo /></div>
    <button class="compose-btn" @click="startCompose('new')"><AppIcon name="plus" />{{ t('side.compose') }}</button>
    <div class="side-scroll">
      <nav class="nav" :aria-label="t('side.folders')">
        <button v-for="r in ROLES" :key="r" class="nav-item" :class="{ active: isActive(r) }" @click="pickRole(r)">
          <AppIcon :name="ROLE_ICONS[r]" />
          <span class="label">{{ ROLE_NAMES[r] }}</span>
          <span class="counts" :title="t('side.countsTitle', { total: fmtCount(totalFor(r, mail.view.account || undefined)), unseen: fmtCount(unseenFor(r, mail.view.account || undefined)) })">
            <span v-if="unseenFor(r, mail.view.account || undefined) && (r === 'inbox' || r === 'junk')" class="count"
                  :class="{ hot: r === 'inbox' }">{{ fmtCount(unseenFor(r, mail.view.account || undefined)) }}</span>
            <span v-if="totalFor(r, mail.view.account || undefined)" class="total">{{ fmtCount(totalFor(r, mail.view.account || undefined)) }}</span>
          </span>
        </button>
        <button class="nav-item" :class="{ active: calendarShown }" @click="openCalendar">
          <AppIcon name="cal" /><span class="label">{{ t('side.calendar') }}</span>
        </button>
        <button class="nav-item" :class="{ active: crmShown }" @click="openCrm">
          <AppIcon name="users" /><span class="label">{{ t('side.contacts') }}</span>
        </button>
      </nav>

      <div>
        <div class="group-label">{{ t('side.mailboxes') }}<button class="grp-add" :title="t('side.newFolder')" :aria-label="t('side.newFolder')" @click="newFolderFor"><AppIcon name="plus" /></button></div>
        <div class="nav">
          <button class="nav-item acct" :class="{ active: !mail.view.account }" @click="pickAccount(null)">
            <span class="dot" style="background:var(--ink-3)"></span>
            <span class="label">{{ t('side.allMailboxes') }}</span>
          </button>
          <template v-for="a in mail.me?.accounts || []" :key="a.address">
            <button class="nav-item acct" :class="{ active: mail.view.account === a.address }" :title="a.address" @click="pickAccount(a.address)">
              <span class="dot" :style="{ background: accountColor(a.address) }"></span>
              <span class="label">{{ a.address }}<span class="acct-sub">{{ a.name || '—' }}{{ a.canSend ? '' : ' · ' + t('side.readOnly') }}</span></span>
              <span v-if="unseenFor('inbox', a.address)" class="count hot">{{ unseenFor('inbox', a.address) }}</span>
            </button>
            <template v-if="mail.view.account === a.address">
              <div v-for="f in customFolders" :key="f.name" class="folder-row">
                <button class="nav-item sub" :class="{ active: mail.view.folder === f.name }" :title="ruleOf(a.address, f.name)?.description || f.name"
                        @click="setView({ account: a.address, folder: f.name })">
                  <AppIcon name="folder" /><span class="label">{{ f.name }}</span>
                  <AppIcon v-if="ruleOf(a.address, f.name)?.auto" name="sparkle" class="auto-ico" />
                  <span class="counts" :title="t('side.countsTitle', { total: fmtCount(f.total), unseen: fmtCount(f.unseen) })">
                    <span v-if="f.unseen" class="count">{{ shortCount(f.unseen) }}</span>
                    <span v-if="f.total" class="total">{{ shortCount(f.total) }}</span>
                  </span>
                </button>
                <button class="folder-edit" :title="t('side.editFolder', { name: f.name })" :aria-label="t('side.editFolder', { name: f.name })" @click="editFolder(a.address, f.name)"><AppIcon name="edit" /></button>
              </div>
              <button class="nav-item sub add" @click="newFolder(a.address)"><AppIcon name="plus" /><span class="label">{{ t('side.newFolder') }}</span></button>
            </template>
          </template>
          <p v-if="mail.me && !mail.me.accounts.length" class="hint" style="padding:6px 10px">
            {{ t('side.noMailboxes') }}
          </p>
        </div>
      </div>

      <div>
        <div class="group-label">{{ t('side.labels') }}<button class="grp-add" :title="t('side.newLabel')" :aria-label="t('side.newLabel')" @click="labels.edit = { id: 0, name: '', color: 'blue' }"><AppIcon name="plus" /></button></div>
        <div class="nav">
          <div v-for="l in labels.list" :key="l.id" class="folder-row">
            <button class="nav-item sub" :class="{ active: mail.view.label === l.id }" @click="setView({ account: mail.view.account, label: l.id })">
              <i class="lbl-dot" :style="{ background: LABEL_COLORS[l.color]?.bg }"></i><span class="label">{{ l.name }}</span>
            </button>
            <button class="folder-edit" :title="t('side.editLabel', { name: l.name })" :aria-label="t('side.editLabel', { name: l.name })" @click="labels.edit = { ...l }"><AppIcon name="edit" /></button>
          </div>
          <p v-if="!labels.list.length" class="hint" style="padding:2px 12px;font-size:12.5px">{{ t('side.labelsHint') }}</p>
        </div>
      </div>
    </div>
    <QuotaMeter />
    <div class="side-foot mobile-only">
      <button class="look-btn" @click="emit('look')">
        <AppIcon name="palette" /><span>{{ t('common.settings') }}</span>
        <span class="sw"><i :style="{ background: sc.l.accent }"></i><i :style="{ background: sc.d.accent }"></i></span>
      </button>
      <NuxtLink v-if="mail.me?.isAdmin" to="/admin" class="look-btn" style="text-decoration:none"><AppIcon name="settings" /><span>{{ t('side.admin') }}</span></NuxtLink>
      <div class="user-row">
        <AppAvatar :login="mail.me?.login" :name="mail.me?.name || mail.me?.login" style="width:32px;height:32px;font-size:12px" />
        <div class="who"><b>{{ mail.me?.name || mail.me?.login }}</b><span>{{ mail.me?.login }}</span></div>
        <button class="icon-btn" :title="t('side.logout')" :aria-label="t('side.logout')" @click="logout"><AppIcon name="logout" /></button>
      </div>
    </div>
  </aside>
</template>
