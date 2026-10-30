<script setup lang="ts">
import { api } from '~/composables/api'
import { ref, computed, watch, nextTick, onBeforeUnmount, inject, type Ref } from 'vue'
import { confirmDialog } from '~/composables/dialogs'
import { labelById, LABEL_COLORS } from '~/composables/labels'
import { newEvent } from '~/composables/calendar'
import { prefetchRow } from '~/composables/mail'
import { mail, visibleMessages, visibleThreads, type Thread, hasMore, loadMessages, loadMore, openMessage, openDraft, isDraftRow, act, keyOf, accountColor, viewTitle, filtersActive, resetFilters, viewTotal, type Summary } from '~/composables/mail'
import { notifyState, requestNotifyPermission, dismissNotifyBanner } from '~/composables/notify'
import { shortTime, displayName, isoDay, ddmm } from '~/composables/format'
import { ext, extColor, canPreview, openAttachment } from '~/composables/attachments'
import { t, fmtDate } from '~/composables/i18n'

const SORTS = computed(() => [
  { value: 'new', label: t('list.sortNew') }, { value: 'old', label: t('list.sortOld') },
  { value: 'from', label: t('list.sortFrom') }, { value: 'unread', label: t('list.sortUnread') },
])
const setListW = inject<(w: number) => void>('setListW')
const listW = inject<Ref<number>>('listW')
function startResize(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  const x0 = e.clientX, w0 = listW?.value || 410
  el.setPointerCapture(e.pointerId)
  document.body.classList.add('resizing')
  const move = (ev: PointerEvent) => setListW?.(w0 + ev.clientX - x0)
  const up = () => { el.removeEventListener('pointermove', move); el.removeEventListener('pointerup', up); document.body.classList.remove('resizing') }
  el.addEventListener('pointermove', move)
  el.addEventListener('pointerup', up)
  e.preventDefault()
}
const rowLabels = (th: Thread) => [...new Set(th.items.flatMap(m => m.labels || []))].filter(id => labelById(id))
const lblStyle = (id: number) => { const c = LABEL_COLORS[labelById(id)?.color || 'gray']; return { background: c?.bg, color: c?.fg } }
const openRow = (m: Summary) => isDraftRow(m) ? openDraft(m) : openMessage(m)
// The pointer rests on a row: fetch the letter ahead, so the click opens it at once.
let hoverTimer: ReturnType<typeof setTimeout> | null = null
function hoverRow(m: Summary | null) {
  if (hoverTimer) clearTimeout(hoverTimer)
  if (m && !isDraftRow(m)) hoverTimer = setTimeout(() => prefetchRow(m), 150)
}
const isCurrent = (th: Thread) => !!mail.selected && (th.items.some(m => keyOf(m) === mail.selected) || !!mail.threadFull[th.key]?.some(m => keyOf(m) === mail.selected))
// "Max, me, Anna" — who wrote in the conversation, oldest first, like Gmail.
// The whole conversation when it was opened (our replies from Sent too), else the rows of this folder.
const convo = (th: Thread) => { const f = mail.threadFull[th.key]; return f && f.length > th.items.length ? f.slice().reverse() : th.items }
function threadNames(th: Thread) {
  const items = convo(th)
  if (items.length === 1) return rowName(th.latest)
  const names: string[] = []
  for (const m of items.slice().reverse()) {
    const n = m.from.email === m.account ? t('list.me') : (displayName(m.from).split(/\s+/)[0] || m.from.email)
    if (!names.includes(n)) names.push(n)
  }
  return names.length > 3 ? `${names[0]} … ${names.slice(-2).join(', ')}` : names.join(', ')
}
const threadFiles = (th: Thread) => th.items.flatMap(m => (m.attachments || []).map(a => ({ m, a })))
const multi = computed(() => (mail.me?.accounts.length || 0) > 1 && !mail.view.account)
const sentView = computed(() => mail.view.role === 'sent' || mail.view.role === 'drafts')

// search: debounce typing, then reload from the server
let qTimer: ReturnType<typeof setTimeout> | null = null
function onSearch(e: Event) {
  mail.filters.q = (e.target as HTMLInputElement).value
  if (qTimer) clearTimeout(qTimer)
  qTimer = setTimeout(loadMessages, 250)
  suggest(mail.filters.q)
}

// Address hints under the search box: contacts matching what is typed (name or address).
type Hit = { email: string, name: string }
const hints = ref<Hit[]>([])
const hintActive = ref(0)
let hintTimer: ReturnType<typeof setTimeout> | null = null
let hintSeq = 0
function suggest(v: string) {
  const term = v.trim()
  if (hintTimer) clearTimeout(hintTimer)
  if (term.length < 2 || /\s/.test(term) && !term.includes('@')) { hints.value = []; return } // a phrase: not an address
  hintTimer = setTimeout(async () => {
    const my = ++hintSeq
    try {
      const res = await api<Hit[]>('/api/contacts', { query: { q: term } })
      if (my !== hintSeq) return
      hints.value = res.filter(h => h.email.toLowerCase() !== term.toLowerCase()).slice(0, 7)
      hintActive.value = 0
    } catch { hints.value = [] }
  }, 150)
}
function pickHint(h: Hit) {
  hintSeq++
  hints.value = []
  mail.filters.q = h.email
  if (qTimer) clearTimeout(qTimer)
  loadMessages()
}
function onSearchKey(e: KeyboardEvent) {
  if (!hints.value.length) {
    if (e.key === 'Enter' && qTimer) { clearTimeout(qTimer); qTimer = null; loadMessages() } // no need to wait out the pause
    return
  }
  if (e.key === 'ArrowDown') { e.preventDefault(); hintActive.value = (hintActive.value + 1) % hints.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); hintActive.value = (hintActive.value - 1 + hints.value.length) % hints.value.length }
  else if (e.key === 'Enter') { e.preventDefault(); pickHint(hints.value[hintActive.value]!) }
  else if (e.key === 'Escape') { hints.value = [] }
}
function hideHints() { setTimeout(() => { hints.value = [] }, 150) }
function toggle(k: 'unseen' | 'att') {
  mail.filters[k] = !mail.filters[k]
  if (k === 'unseen') loadMessages()
}

// period popover
const periodOpen = ref(false)
const periodBtn = ref<HTMLElement | null>(null)
const pop = ref<HTMLElement | null>(null)
const pFrom = ref(''), pTo = ref(''), pErr = ref('')
const popStyle = ref<Record<string, string>>({})
const today = isoDay(new Date())
async function openPeriod() {
  if (periodOpen.value) { periodOpen.value = false; return }
  pFrom.value = mail.filters.since; pTo.value = mail.filters.until; pErr.value = ''
  periodOpen.value = true
  await nextTick()
  const r = periodBtn.value!.getBoundingClientRect()
  popStyle.value = { left: `${Math.max(12, Math.min(r.left, innerWidth - 352))}px`, top: `${r.bottom + 6}px` }
  document.addEventListener('mousedown', outside)
}
function outside(e: MouseEvent) {
  if (pop.value && !pop.value.contains(e.target as Node) && !periodBtn.value?.contains(e.target as Node)) closePeriod()
}
function closePeriod() { periodOpen.value = false; document.removeEventListener('mousedown', outside) }
onBeforeUnmount(() => document.removeEventListener('mousedown', outside))
const PRESETS = computed(() => [['today', t('date.today')], ['yesterday', t('date.yesterday')], ['7', t('list.lastDays', { n: 7 })], ['30', t('list.lastDays', { n: 30 })], ['month', t('list.thisMonth')], ['prev', t('list.lastMonth')]] as const)
const PRESET_KEYS = ['today', 'yesterday', '7', '30', 'month', 'prev'] as const
function presetRange(kind: string): [string, string] {
  const d = new Date(), from = new Date()
  if (kind === 'yesterday') { from.setDate(d.getDate() - 1); d.setDate(d.getDate() - 1) }
  if (kind === '7') from.setDate(d.getDate() - 6)
  if (kind === '30') from.setDate(d.getDate() - 29)
  if (kind === 'month') from.setDate(1)
  if (kind === 'prev') { from.setDate(1); from.setMonth(from.getMonth() - 1); d.setDate(0) }
  return [isoDay(from), isoDay(d)]
}
function preset(kind: string) { [pFrom.value, pTo.value] = presetRange(kind); pErr.value = '' }
const activePreset = computed(() => PRESET_KEYS.find(k => { const [a, b] = presetRange(k); return a === pFrom.value && b === pTo.value }) || '')
const pSummary = computed(() => {
  if (!pFrom.value && !pTo.value) return t('list.allTime')
  const f = (iso: string) => fmtDate(`${iso}T12:00:00Z`, { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })
  if (pFrom.value === pTo.value) return f(pFrom.value)
  if (!pTo.value) return t('list.rangeFromToday', { from: f(pFrom.value) })
  return t('list.range', { from: pFrom.value ? f(pFrom.value) : '…', to: f(pTo.value) })
})
const activeFilterNames = computed(() => {
  const f = mail.filters, n: string[] = []
  if (f.q) n.push(t('list.filterSearch', { q: f.q }))
  if (f.unseen) n.push(t('list.filterUnread'))
  if (f.att) n.push(t('list.filterAtt'))
  if (f.since || f.until) n.push(t('list.filterPeriod'))
  return n.join(', ') || t('list.filterAny')
})
function applyPeriod(reset = false) {
  if (reset) { pFrom.value = ''; pTo.value = '' }
  if (pFrom.value && pTo.value && pFrom.value > pTo.value) { pErr.value = t('list.periodOrder'); return }
  mail.filters.since = pFrom.value; mail.filters.until = pTo.value
  closePeriod()
  loadMessages()
}
const periodLabel = computed(() => (mail.filters.since || mail.filters.until) ? `${ddmm(mail.filters.since)} – ${ddmm(mail.filters.until)}` : t('list.period'))

function rowName(m: Summary) {
  if (sentView.value) return t('list.toNames', { names: m.to.map(a => a.name || a.email).join(', ') || '—' })
  return displayName(m.from)
}
function toggleStar(m: Summary) { act(m.flagged ? 'unflag' : 'flag', [m]) }

// ---------- selection and bulk actions (like Gmail) ----------
const checked = ref(new Set<string>())
watch(() => [mail.view.account, mail.view.role, mail.view.folder, mail.filters.q], () => checked.value.clear())
watch(visibleThreads, ts => {
  const keys = new Set(ts.map(th => th.key))
  for (const k of [...checked.value]) if (!keys.has(k)) checked.value.delete(k)
})
const selThreads = computed(() => visibleThreads.value.filter(th => checked.value.has(th.key)))
const selItems = computed(() => selThreads.value.flatMap(th => th.items))
const allChecked = computed(() => visibleThreads.value.length > 0 && selThreads.value.length === visibleThreads.value.length)
function toggleCheck(th: Thread) {
  const s = new Set(checked.value)
  if (s.has(th.key)) s.delete(th.key); else s.add(th.key)
  checked.value = s
}
function selectBy(kind: 'all' | 'none' | 'read' | 'unread' | 'flagged' | 'unflagged') {
  const pick: Record<string, (th: Thread) => boolean> = {
    all: () => true, none: () => false, read: th => th.items.every(x => x.seen), unread: th => th.items.some(x => !x.seen),
    flagged: th => th.latest.flagged, unflagged: th => !th.latest.flagged,
  }
  checked.value = new Set(visibleThreads.value.filter(pick[kind]).map(th => th.key))
}
function toggleAll() { selectBy(allChecked.value || checked.value.size ? 'none' : 'all') }
const roleOfView = computed(() => mail.view.folder ? (Object.values(mail.folders).flat().find(f => f.name === mail.view.folder)?.role || '') : mail.view.role)
const inTrash = computed(() => roleOfView.value === 'trash')
const inJunk = computed(() => roleOfView.value === 'junk')
async function bulk(action: 'seen' | 'unseen' | 'flag' | 'unflag' | 'move' | 'delete', target?: string) {
  const items = selItems.value
  if (!items.length) return
  if (action === 'delete' && inTrash.value && !await confirmDialog(t('list.deleteForeverN', { n: selThreads.value.length }), t('list.deleteForeverManyText'), { ok: t('common.delete'), danger: true })) return
  await act(action, items, target)
  if (action === 'move' || action === 'delete') checked.value = new Set()
}
const someUnread = computed(() => selItems.value.some(x => !x.seen))
const moveTargets = computed(() => {
  const accs = new Set(selItems.value.map(x => x.account))
  if (accs.size !== 1) return []
  const acc = [...accs][0]
  return (mail.folders[acc] || []).filter(f => (!f.role || f.role === 'archive' || f.role === 'inbox') && (mail.view.folder ? f.name !== mail.view.folder : f.role !== mail.view.role))
})

// Hover buttons on a row.
async function rowAct(th: Thread, action: 'seen' | 'unseen' | 'move' | 'delete', target?: string) {
  if (action === 'delete' && inTrash.value && !await confirmDialog(t('list.deleteForeverOne'), t('list.deleteForeverOneText'), { ok: t('common.delete'), danger: true })) return
  act(action, th.items, target)
}
function remindLater(th: Thread) {
  const d = new Date(); d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0)
  newEvent(d, { title: t('list.remindTitle', { subject: th.latest.subject || t('common.noSubject') }), notes: t('list.remindNotes', { name: th.latest.from.name || th.latest.from.email }), mailRef: keyOf(th.latest), remindMinutes: 0, color: 'yellow' })
}

const showBanner = computed(() => notifyState.supported && notifyState.enabled && notifyState.permission === 'default' && !notifyState.bannerDismissed)

function onScroll(e: Event) {
  const el = e.target as HTMLElement
  if (hasMore.value && !mail.loadingMore && el.scrollTop + el.clientHeight > el.scrollHeight - 600) loadMore()
}
</script>

<template>
  <section class="list" :aria-label="t('list.ariaList')">
    <div class="col-resize" role="separator" aria-orientation="vertical" :aria-label="t('list.ariaWidth')" :title="t('list.resizeHint')"
         @pointerdown="startResize" @dblclick="setListW?.(410)"></div>
    <div v-if="showBanner" class="banner">
      <AppIcon name="bell" />
      <span>{{ t('list.notifyBanner') }}</span>
      <span class="grow"></span>
      <button class="btn sm primary" @click="requestNotifyPermission">{{ t('list.notifyEnable') }}</button>
      <button class="btn sm" @click="dismissNotifyBanner">{{ t('list.notNow') }}</button>
    </div>
    <div v-if="!mail.online" class="banner" style="background:var(--warn-soft);border-color:transparent">
      <AppIcon name="wifiOff" /><span>{{ t('list.offline') }}</span>
    </div>
    <div class="list-head">
      <div class="list-title">
        <button class="icon-btn menu-btn" :aria-label="t('list.openFolders')" @click="mail.sideOpen = true"><AppIcon name="menu" /></button>
        <h1>{{ viewTitle }}</h1>
        <label class="sort" :title="t('list.sort')">
          <AppSelect v-model="mail.filters.sort" :options="SORTS" small :aria-label="t('list.sort')" />
        </label>
      </div>
      <label class="search">
        <AppIcon name="search" />
        <span class="sr-only">{{ t('common.search') }}</span>
        <input :value="mail.filters.q" type="search" :placeholder="t('list.searchPlaceholder')" autocomplete="off" role="combobox" :aria-expanded="hints.length > 0"
               aria-controls="search-hints" @input="onSearch" @keydown="onSearchKey" @blur="hideHints" />
        <ul v-if="hints.length" id="search-hints" class="search-hints" role="listbox">
          <li v-for="(h, i) in hints" :key="h.email" role="option" :aria-selected="i === hintActive" :class="{ on: i === hintActive }"
              @mousedown.prevent="pickHint(h)" @mouseenter="hintActive = i">
            <AppAvatar :email="h.email" :name="h.name || h.email" style="width:26px;height:26px;font-size:11px" />
            <span class="sh-txt"><b>{{ h.name || h.email }}</b><span v-if="h.name">{{ h.email }}</span></span>
          </li>
        </ul>
      </label>
      <div class="bulk-bar" :class="{ active: checked.size }">
        <span class="bulk-check">
          <input type="checkbox" :checked="allChecked" :indeterminate.prop="checked.size > 0 && !allChecked" :aria-label="t('list.selectAll')" @change="toggleAll" />
          <ToolPop v-slot="{ close }" :title="t('list.select')" icon="chev" :down="true">
            <button class="tp-item" @click="close(); selectBy('all')">{{ t('common.all') }}</button>
            <button class="tp-item" @click="close(); selectBy('none')">{{ t('list.selNone') }}</button>
            <button class="tp-item" @click="close(); selectBy('read')">{{ t('list.selRead') }}</button>
            <button class="tp-item" @click="close(); selectBy('unread')">{{ t('list.selUnread') }}</button>
            <button class="tp-item" @click="close(); selectBy('flagged')">{{ t('list.selFlagged') }}</button>
            <button class="tp-item" @click="close(); selectBy('unflagged')">{{ t('list.selUnflagged') }}</button>
          </ToolPop>
        </span>
        <template v-if="checked.size">
          <span class="bulk-n">{{ checked.size }}</span>
          <button v-if="inTrash" class="icon-btn" :title="t('list.restoreToInbox')" :aria-label="t('list.restore')" @click="bulk('move', 'inbox')"><AppIcon name="undo" /></button>
          <button v-else-if="roleOfView !== 'archive'" class="icon-btn" :title="t('list.archive')" :aria-label="t('list.archive')" @click="bulk('move', 'archive')"><AppIcon name="archive" /></button>
          <button v-if="inJunk" class="icon-btn" :title="t('list.notSpam')" :aria-label="t('list.notSpam')" @click="bulk('move', 'inbox')"><AppIcon name="check" /></button>
          <button v-else-if="!inTrash" class="icon-btn" :title="t('list.isSpam')" :aria-label="t('list.isSpam')" @click="bulk('move', 'junk')"><AppIcon name="junk" /></button>
          <button class="icon-btn" :title="inTrash ? t('list.deleteForever') : t('common.delete')" :aria-label="inTrash ? t('list.deleteForever') : t('common.delete')" @click="bulk('delete')"><AppIcon name="trash" /></button>
          <button class="icon-btn" :title="someUnread ? t('list.markReadMany') : t('list.markUnreadMany')" :aria-label="someUnread ? t('list.markReadMany') : t('list.markUnreadMany')" @click="bulk(someUnread ? 'seen' : 'unseen')"><AppIcon :name="someUnread ? 'mailOpen' : 'mail'" /></button>
          <button class="icon-btn" :title="t('list.flag')" :aria-label="t('list.flag')" @click="bulk(selItems.every(x => x.flagged) ? 'unflag' : 'flag')"><AppIcon name="star" /></button>
          <ToolPop v-if="moveTargets.length" v-slot="{ close }" :title="t('list.moveToFolder')" icon="folder" :down="true">
            <div class="move-list">
              <button v-for="f in moveTargets" :key="f.name" class="tp-item" @click="close(); bulk('move', 'folder:' + f.name)">
                <AppIcon :name="f.role === 'archive' ? 'archive' : f.role === 'inbox' ? 'inbox' : 'folder'" /> {{ f.role === 'archive' ? t('list.folderArchive') : f.role === 'inbox' ? t('list.folderInbox') : f.name }}</button>
            </div>
          </ToolPop>
          <LabelPicker :items="selItems" />
          <button class="link bulk-clear" @click="checked = new Set()">{{ t('list.clearSelection') }}</button>
        </template>
      </div>
      <div v-show="!checked.size" class="chips">
        <button class="chip" :class="{ on: mail.filters.unseen }" :aria-pressed="mail.filters.unseen" @click="toggle('unseen')"><span class="chip-dot" />{{ t('list.chipUnread') }}</button>
        <button class="chip" :class="{ on: mail.filters.att }" :aria-pressed="mail.filters.att" @click="toggle('att')"><AppIcon name="clip" />{{ t('list.chipAtt') }}</button>
        <button ref="periodBtn" class="chip" :class="{ on: mail.filters.since || mail.filters.until }" aria-haspopup="dialog" @click="openPeriod">
          <AppIcon name="cal" /><span class="tnum">{{ periodLabel }}</span>
          <span v-if="mail.filters.since || mail.filters.until" class="x" role="button" :aria-label="t('list.resetPeriod')" @click.stop="applyPeriod(true)"><AppIcon name="close" /></span>
          <AppIcon v-else name="chev" class="chip-chev" :class="{ up: periodOpen }" />
        </button>
      </div>
    </div>

    <div class="rows-wrap veil-host">
    <LoadingVeil :show="mail.listVeil" :text="mail.filters.q ? t('list.searching') : t('list.loading')" />
    <div class="rows" @scroll.passive="onScroll">
      <div v-if="mail.searchPartial && mail.filters.q" class="search-partial" role="status">
        <AppSpinner size="sm" />{{ mail.searchIndexed ? t('list.searchingAttachments', { q: mail.filters.q }) : t('list.searchingBodies', { q: mail.filters.q }) }}
      </div>
      <div v-if="mail.loading && !mail.messages.length" class="rows-empty" aria-busy="true"></div>
      <div v-else-if="mail.listError" class="empty">
        <p>{{ mail.listError }}</p>
        <button class="btn" @click="loadMessages()"><AppIcon name="refresh" />{{ t('common.retry') }}</button>
      </div>
      <div v-else-if="!visibleMessages.length" class="empty">
        <template v-if="filtersActive || mail.filters.att">
          <p style="margin:0 0 10px">{{ viewTotal() ? t('list.hiddenByFilters', { n: viewTotal(), filters: activeFilterNames }) : t('list.nothingFound', { filters: activeFilterNames }) }}</p>
          <button class="btn sm" @click="resetFilters"><AppIcon name="close" />{{ t('list.showAll') }}</button>
        </template>
        <template v-else-if="mail.searchPartial">{{ mail.searchIndexed ? t('list.notInLettersYet') : t('list.notInSubjectsYet') }}</template>
        <template v-else>{{ t('list.empty') }}</template>
      </div>
      <template v-else>
        <div v-for="th in visibleThreads" :key="th.key" class="row"
             :class="{ unread: th.items.some(x => !x.seen), current: isCurrent(th), loading: isCurrent(th) && mail.currentLoading, fresh: th.latest.fresh, checked: checked.has(th.key), selecting: checked.size > 0 }"
             tabindex="0" role="button" :aria-label="th.latest.subject" @click="openRow(th.latest)" @keydown.enter="openRow(th.latest)"
             @mouseenter="hoverRow(th.latest)" @mouseleave="hoverRow(null)">
          <span class="row-pick" @click.stop="toggleCheck(th)">
            <AppAvatar v-if="sentView" :email="th.latest.to[0]?.email" :name="th.latest.to[0]?.name || th.latest.to[0]?.email" />
            <AppAvatar v-else :email="th.latest.from.email" :name="displayName(th.latest.from)" :bimi="th.latest.dmarc === 'pass'" />
            <input type="checkbox" class="row-cb" :checked="checked.has(th.key)" :aria-label="t('list.selectRow', { subject: th.latest.subject || t('list.aLetter') })" tabindex="-1" />
          </span>
          <div class="row-main">
            <div class="row-from"><b>{{ threadNames(th) }}</b><span v-if="convo(th).length > 1" class="row-count" :title="t('list.inThread', { n: convo(th).length })">{{ convo(th).length }}</span></div>
            <div class="row-subj"><span v-for="id in rowLabels(th)" :key="id" class="lbl" :style="lblStyle(id)">{{ labelById(id)?.name }}</span>{{ th.latest.subject || t('common.noSubject') }}</div>
            <div class="row-prev">{{ th.latest.preview }}</div>
            <div v-if="threadFiles(th).length" class="row-files">
              <button v-for="f in threadFiles(th).slice(0, 3)" :key="keyOf(f.m) + f.a.part" type="button" class="file-pill"
                      :title="canPreview(f.a) ? t('list.fileView', { name: f.a.name }) : t('list.fileDownload', { name: f.a.name })" @click.stop="openAttachment(f.m, f.a, f.m.attachments)" @keydown.enter.stop>
                <span class="fp-ext" :style="{ background: extColor(f.a) }">{{ ext(f.a) || 'FILE' }}</span><span class="fp-name">{{ f.a.name }}</span>
              </button>
              <button v-if="threadFiles(th).length > 3" type="button" class="file-pill more" :title="t('list.moreFiles', { n: threadFiles(th).length - 3 })" @click.stop="openRow(th.latest)" @keydown.enter.stop>+{{ threadFiles(th).length - 3 }}</button>
            </div>
            <div v-if="multi || th.latest.sentBy" class="tags">
              <span v-if="multi" class="to-pill"><span class="dot" :style="{ background: accountColor(th.latest.account) }"></span>{{ th.latest.account }}</span>
              <span v-if="th.latest.sentBy" class="tag" :title="th.latest.sentBy.name">{{ t('list.sentBy', { login: th.latest.sentBy.login }) }}</span>
            </div>
          </div>
          <div class="row-hover" @click.stop>
            <button v-if="inTrash" class="icon-btn" :title="t('list.restore')" :aria-label="t('list.restore')" @click="rowAct(th, 'move', 'inbox')"><AppIcon name="undo" /></button>
            <button v-else-if="roleOfView !== 'archive'" class="icon-btn" :title="t('list.archive')" :aria-label="t('list.archive')" @click="rowAct(th, 'move', 'archive')"><AppIcon name="archive" /></button>
            <button class="icon-btn" :title="inTrash ? t('list.deleteForever') : t('common.delete')" :aria-label="inTrash ? t('list.deleteForever') : t('common.delete')" @click="rowAct(th, 'delete')"><AppIcon name="trash" /></button>
            <button class="icon-btn" :title="th.items.some(x => !x.seen) ? t('list.markRead') : t('list.markUnread')" :aria-label="t('list.readUnread')"
                    @click="rowAct(th, th.items.some(x => !x.seen) ? 'seen' : 'unseen')"><AppIcon :name="th.items.some(x => !x.seen) ? 'mailOpen' : 'mail'" /></button>
            <button class="icon-btn" :title="t('list.remindTomorrow', { time: fmtDate(Date.UTC(2024, 0, 1, 9), { hour: 'numeric', minute: '2-digit', timeZone: 'UTC' }) })" :aria-label="t('list.remind')" @click="remindLater(th)"><AppIcon name="clock" /></button>
          </div>
          <div class="row-meta">
            <span class="row-time" :title="th.latest.date">{{ shortTime(th.latest.date) }}</span>
            <span class="row-icons">
              <AppIcon v-if="th.latest.answered" name="reply" />
              <AppIcon v-if="th.items.some(x => x.hasAttachments)" name="clip" />
              <button class="star" :class="{ on: th.latest.flagged }" :aria-label="th.latest.flagged ? t('list.unflag') : t('list.flag')" @click.stop="toggleStar(th.latest)">
                <AppIcon name="star" :fill="th.latest.flagged" />
              </button>
            </span>
          </div>
        </div>
        <div v-if="hasMore" class="more">
          <div v-if="mail.loadingMore" class="more-loading" role="status"><AppSpinner />{{ t('list.loadingMore') }}</div>
          <button v-else class="btn" @click="loadMore()">{{ t('list.showMore') }}</button>
        </div>
      </template>
    </div>
    </div>

    <Teleport to="body">
      <div v-if="periodOpen" ref="pop" class="pop pop-period" role="dialog" :aria-label="t('list.period')" :style="popStyle">
        <div class="pp-head">
          <span class="pp-ico"><AppIcon name="cal" /></span>
          <div class="pp-title"><b>{{ t('list.period') }}</b><span>{{ pSummary }}</span></div>
          <button class="icon-btn" :aria-label="t('common.close')" @click="closePeriod"><AppIcon name="close" /></button>
        </div>
        <div class="period">
          <div class="presets">
            <button v-for="[k, label] in PRESETS" :key="k" :class="{ on: activePreset === k }" @click="preset(k)">{{ label }}</button>
          </div>
          <div class="dates">
            <label><span>{{ t('list.dateFrom') }}</span><DateInput v-model="pFrom" :max="today" :aria-label="t('list.dateFrom')" /></label>
            <span class="dash">—</span>
            <label><span>{{ t('list.dateTo') }}</span><DateInput v-model="pTo" :max="today" :aria-label="t('list.dateTo')" /></label>
          </div>
          <p v-if="pErr" class="err">{{ pErr }}</p>
        </div>
        <div class="pp-foot">
          <button class="btn ghost" :disabled="!pFrom && !pTo" @click="applyPeriod(true)">{{ t('common.reset') }}</button>
          <button class="btn primary" @click="applyPeriod()">{{ t('list.showLetters') }}</button>
        </div>
      </div>
    </Teleport>
  </section>
</template>
