<script setup lang="ts">
// Home: a greeting, unread letters per mailbox, letters waiting to be sent or snoozed, the next
// seven days of the calendar (tasks first) and the latest files, each one click away.
import { computed, onMounted } from 'vue'
import { home, loadHome, type HomeFile } from '~/composables/home'
import { mail, setView, startCompose, accountColor, ROLE_NAMES } from '~/composables/mail'
import { cal, newEvent, CAL_COLORS, evKey, toggleDone, type CalEvent } from '~/composables/calendar'
import { later } from '~/composables/later'
import { ext, extColor, openAttachment } from '~/composables/attachments'
import { fileSize, displayName } from '~/composables/format'
import { t, fmtDate } from '~/composables/i18n'

onMounted(() => loadHome())
const hour = new Date().getHours()
const greeting = computed(() => t(hour < 5 ? 'home.night' : hour < 12 ? 'home.morning' : hour < 18 ? 'home.day' : 'home.evening',
  { name: firstName(mail.me?.name || mail.me?.login || '') }))
// "Иванов Андрей Сергеевич" (surname, name, patronymic) → "Андрей"; "Anna Smith" → "Anna".
function firstName(full: string) {
  const w = full.trim().split(/\s+/)
  return w.length >= 3 && /(вич|вна|ична|оглы|кызы)$/i.test(w[2]!) ? w[1]! : w[0]!
}
const today = computed(() => fmtDate(new Date(), { weekday: 'long', day: 'numeric', month: 'long' }))
const boxes = computed(() => (mail.me?.accounts || []).map(a => ({
  address: a.address, name: a.name,
  unread: mail.folders[a.address]?.find(f => f.role === 'inbox')?.unseen || 0,
  total: mail.folders[a.address]?.find(f => f.role === 'inbox')?.total || 0,
})))
const totalUnread = computed(() => boxes.value.reduce((n, b) => n + b.unread, 0))
function openBox(address: string) { home.open = false; setView({ account: address, role: 'inbox' }); mail.reading = false }
function openRole(role: string) { home.open = false; setView({ role }); mail.reading = false }

// The next seven days, by day; undone tasks of the past week come first as "overdue".
const days = computed(() => {
  const by: Record<string, { date: Date, items: CalEvent[] }> = {}
  const start = new Date(); start.setHours(0, 0, 0, 0)
  for (const e of home.events) {
    const d = new Date(e.start); d.setHours(0, 0, 0, 0)
    if (d < start && !(e.isTask && !e.done)) continue
    const key = (d < start ? start : d).toDateString()
    ;(by[key] ||= { date: d < start ? start : d, items: [] }).items.push(e)
  }
  return Object.values(by).sort((a, b) => +a.date - +b.date).map(x => ({ ...x, items: x.items.sort((a, b) => Number(b.allDay) - Number(a.allDay) || a.start.localeCompare(b.start)) }))
})
const dayLabel = (d: Date) => {
  const n = new Date(); n.setHours(0, 0, 0, 0)
  const diff = Math.round((+d - +n) / 864e5)
  return diff === 0 ? t('date.today') : diff === 1 ? t('home.tomorrow') : fmtDate(d, { weekday: 'long', day: 'numeric', month: 'short' })
}
const timeOf = (e: CalEvent) => e.allDay ? t('home.allDay') : fmtDate(e.start, { hour: '2-digit', minute: '2-digit' })
async function editEvent(e: CalEvent) { cal.editing = { ...e } }
function addEvent() { newEvent() }

function openFile(f: HomeFile) { openAttachment({ account: f.account, folder: f.folder, uid: f.uid }, f.file, home.files.filter(x => x.account === f.account && x.uid === f.uid && x.folder === f.folder).map(x => x.file)) }
const fileWho = (f: HomeFile) => f.sent ? t('home.sentFile') : displayName(f.from)
const fileDate = (iso: string) => fmtDate(iso, { day: 'numeric', month: 'short' })
</script>

<template>
  <div class="home">
    <header class="home-head">
      <button class="icon-btn back-btn" :aria-label="t('calendar.backToMail')" @click="mail.reading = false"><AppIcon name="back" /></button>
      <div class="home-hello">
        <h2>{{ greeting }}</h2>
        <span class="hint">{{ today }} · {{ totalUnread ? t('home.unreadTotal', { n: totalUnread }) : t('home.allRead') }}</span>
      </div>
      <span class="grow"></span>
      <button class="btn sm" @click="addEvent"><AppIcon name="cal" />{{ t('calendar.event') }}</button>
      <button class="btn sm primary" @click="startCompose('new')"><AppIcon name="plus" />{{ t('side.compose') }}</button>
    </header>

    <div class="home-grid">
      <section class="home-card">
        <h3><AppIcon name="inbox" />{{ t('home.mail') }}</h3>
        <button v-for="b in boxes" :key="b.address" type="button" class="home-box" @click="openBox(b.address)">
          <i class="dot" :style="{ background: accountColor(b.address) }"></i>
          <span class="hb-main"><b>{{ b.address }}</b><span v-if="b.name" class="hint">{{ b.name }}</span></span>
          <span v-if="b.unread" class="count hot">{{ b.unread }}</span>
          <span v-else class="hint hb-ok"><AppIcon name="check" /></span>
        </button>
        <div v-if="later.scheduled.length || later.snoozed.length" class="home-wait">
          <button v-if="later.scheduled.length" type="button" class="chip" @click="openRole('scheduled')"><AppIcon name="sendClock" />{{ ROLE_NAMES.scheduled }} · {{ later.scheduled.length }}</button>
          <button v-if="later.snoozed.length" type="button" class="chip" @click="openRole('snoozed')"><AppIcon name="clock" />{{ ROLE_NAMES.snoozed }} · {{ later.snoozed.length }}</button>
        </div>
      </section>

      <section class="home-card veil-host">
        <LoadingVeil :show="home.eventsLoading" :text="t('calendar.loading')" />
        <h3><AppIcon name="cal" />{{ t('home.week') }}</h3>
        <p v-if="!days.length && !home.eventsLoading" class="home-empty hint">{{ t('home.weekEmpty') }}</p>
        <div v-for="d in days" :key="d.date.toDateString()" class="home-day">
          <div class="hd-label">{{ dayLabel(d.date) }}</div>
          <div v-for="e in d.items" :key="evKey(e)" class="home-ev" :class="{ done: e.done, overdue: e.isTask && !e.done && new Date(e.start) < new Date(new Date().setHours(0, 0, 0, 0)) }">
            <input v-if="e.isTask" type="checkbox" :checked="e.done" :aria-label="t('calendar.doneAria', { title: e.title })" @change="toggleDone(e); e.done = !e.done" />
            <i v-else class="dot" :style="{ background: CAL_COLORS[e.color]?.bg }"></i>
            <button type="button" class="he-main" @click="editEvent(e)"><span class="he-time">{{ timeOf(e) }}</span><span class="he-title">{{ e.title }}</span></button>
            <AppIcon v-if="e.rrule" name="repeat" class="he-flag" />
          </div>
        </div>
      </section>

      <section class="home-card wide veil-host">
        <LoadingVeil :show="home.filesLoading" :text="t('home.filesLoading')" />
        <h3><AppIcon name="clip" />{{ t('home.files') }}<span class="grow"></span>
          <button type="button" class="icon-btn" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="loadHome(true)"><AppIcon name="refresh" /></button></h3>
        <p v-if="!home.files.length && !home.filesLoading" class="home-empty hint">{{ t('home.filesEmpty') }}</p>
        <div class="home-files">
          <button v-for="f in home.files" :key="f.account + f.folder + f.uid + f.file.part" type="button" class="home-file" :title="f.subject" @click="openFile(f)">
            <span class="hf-ext" :style="{ background: extColor(f.file) }">{{ ext(f.file) || 'FILE' }}</span>
            <span class="hf-txt"><b>{{ f.file.name }}</b><span class="hint">{{ fileWho(f) }} · {{ fileDate(f.date) }} · {{ fileSize(f.file.size) }}</span></span>
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.home{display:flex;flex-direction:column;height:100%;min-height:0;overflow:auto;background:var(--surface-2)}
.home-head{display:flex;align-items:center;gap:10px;padding:18px 22px 10px;flex-wrap:wrap}
.home-hello h2{margin:0;font-size:22px;font-weight:600;letter-spacing:-.01em}
.home-hello .hint{font-size:13px;text-transform:none}
.home-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;padding:8px 22px 22px}
.home-card{background:var(--surface);border:1px solid var(--line);border-radius:6px;padding:14px 16px;display:flex;flex-direction:column;gap:6px;min-width:0;min-height:120px}
.home-card.wide{grid-column:1 / -1}
.home-card h3{display:flex;align-items:center;gap:8px;margin:0 0 4px;font-size:14px;font-weight:600}
.home-card h3 svg{width:17px;height:17px;color:var(--accent)}
.home-card h3 .icon-btn{width:28px;height:28px}
.home-box{display:flex;align-items:center;gap:10px;padding:8px 8px;border:0;border-radius:5px;background:transparent;cursor:pointer;text-align:left;color:var(--ink)}
.home-box:hover{background:var(--surface-3)}
.home-box .dot,.home-ev .dot{width:9px;height:9px;border-radius:50%;flex:none}
.hb-main{display:flex;flex-direction:column;min-width:0;flex:1}
.hb-main b{font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.hb-main .hint{font-size:12px}
.hb-ok svg{width:15px;height:15px;color:var(--ok)}
.home-box .count{min-width:24px;text-align:center}
.home-wait{display:flex;gap:6px;flex-wrap:wrap;padding-top:6px;border-top:1px solid var(--line);margin-top:4px}
.home-wait .chip svg{width:14px;height:14px}
.home-empty{margin:6px 0;font-size:13px}
.home-day{display:flex;flex-direction:column;gap:2px;padding:4px 0}
.hd-label{font-size:12px;font-weight:600;color:var(--ink-3);padding:2px 8px}
.hd-label::first-letter{text-transform:uppercase}
.home-ev{display:flex;align-items:center;gap:8px;padding:3px 8px;border-radius:4px}
.home-ev:hover{background:var(--surface-3)}
.home-ev input{margin:0;accent-color:var(--accent)}
.home-ev.done .he-title{text-decoration:line-through;color:var(--ink-3)}
.home-ev.overdue .he-time{color:var(--danger)}
.he-main{display:flex;gap:10px;align-items:baseline;min-width:0;flex:1;border:0;background:none;padding:2px 0;cursor:pointer;text-align:left;color:var(--ink)}
.he-time{font-size:12.5px;color:var(--ink-3);font-variant-numeric:tabular-nums;flex:none;min-width:44px}
.he-title{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.he-flag{width:12px;height:12px;color:var(--ink-3)}
.home-files{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:8px}
.home-file{display:flex;align-items:center;gap:10px;padding:8px 10px;border:1px solid var(--line);border-radius:5px;background:var(--surface);cursor:pointer;text-align:left;min-width:0;color:var(--ink)}
.home-file:hover{border-color:var(--accent-line);background:var(--accent-soft)}
.hf-ext{flex:none;width:40px;height:40px;border-radius:5px;display:grid;place-items:center;color:#fff;font-size:11px;font-weight:700;letter-spacing:.02em}
.hf-txt{display:flex;flex-direction:column;min-width:0}
.hf-txt b{font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.hf-txt .hint{font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
@media (max-width:900px){.home-grid{grid-template-columns:1fr;padding:8px 12px 16px}.home-head{padding:14px 12px 8px}}
</style>
