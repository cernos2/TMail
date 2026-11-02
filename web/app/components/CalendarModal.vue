<script setup lang="ts">
// A calendar's settings: name and colour, colleagues who see or edit it, the subscription link
// for Google / Apple / Outlook, and how to see it on a phone.
import { ref, computed, onMounted } from 'vue'
import { api } from '~/composables/api'
import { cal, saveCalendar, deleteCalendar, CAL_COLORS, calName, type Calendar } from '~/composables/calendar'
import { mail } from '~/composables/mail'
import { toast } from '~/composables/toast'
import { confirmDialog, copyText } from '~/composables/dialogs'
import { t } from '~/composables/i18n'

const src = cal.calEditing!
const isNew = !src.id
const f = ref({ name: src.name || '', color: src.color || 'blue' })
const shares = ref((src.shares || []).map(x => ({ ...x })))
const icsUrl = ref(src.icsUrl || '')
const people = ref<{ userId: number, login: string, name: string }[]>([])
const addWho = ref<number | ''>('')
const busy = ref(false)
const err = ref('')
const personal = computed(() => !isNew && !src.name && cal.calendars.filter(c => c.mine)[0]?.id === src.id)
onMounted(async () => { try { people.value = await api('/api/calendars/people') } catch {} })
const free = computed(() => people.value.filter(p => !shares.value.some(s => s.userId === p.userId))
  .map(p => ({ value: p.userId as number | string, label: p.name ? `${p.name} (${p.login})` : p.login })))
const freeOptions = computed(() => [{ value: '' as number | string, label: t('calendar.pickColleague') }, ...free.value])
function addShare() {
  const p = people.value.find(x => x.userId === addWho.value)
  if (p) shares.value.push({ userId: p.userId, login: p.login, name: p.name, canWrite: false })
  addWho.value = ''
}
const colorLabel = (k: string | number) => t('calendar.color' + String(k).charAt(0).toUpperCase() + String(k).slice(1))

async function save() {
  if (!f.value.name.trim() && !personal.value) { err.value = t('calendar.nameRequired'); return }
  busy.value = true
  err.value = ''
  try {
    await saveCalendar({ id: src.id, name: personal.value ? '' : f.value.name.trim(), color: f.value.color, shares: isNew ? undefined : shares.value })
    toast(isNew ? t('calendar.calendarCreated') : t('common.saved'))
    cal.calEditing = null
  } catch (e: any) { err.value = e.message } finally { busy.value = false }
}
async function setIcs(on: boolean, rotate = false) {
  if (!on && !await confirmDialog(t('calendar.icsOffConfirm'), t('calendar.icsOffText'), { ok: t('calendar.icsOff'), danger: true })) return
  if (rotate && !await confirmDialog(t('calendar.icsRotateConfirm'), t('calendar.icsRotateText'), { ok: t('calendar.icsRotate') })) return
  busy.value = true
  try {
    const r = await saveCalendar({ id: src.id, ics: on, rotate })
    icsUrl.value = on ? (r as any).icsUrl || '' : ''
  } catch (e: any) { toast(e.message, { kind: 'err' }) } finally { busy.value = false }
}
async function remove() {
  if (!await confirmDialog(t('calendar.deleteCalendarConfirm', { name: calName(src as Calendar) }), t('calendar.deleteCalendarText'), { ok: t('common.delete'), danger: true })) return
  try { await deleteCalendar(src as Calendar); toast(t('calendar.calendarDeleted')); cal.calEditing = null } catch (e: any) { err.value = e.message }
}
async function copy(v: string) { if (await copyText(v)) toast(t('common.copied')) }
function openDevices() { cal.calEditing = null; mail.settingsTab = 'clients' }
</script>

<template>
  <AppModal :title="isNew ? t('calendar.newCalendar') : t('calendar.calendarSettings')" width="560px" :busy="busy ? t('common.saving') : ''" @close="cal.calEditing = null">
    <form class="form calm" @submit.prevent="save">
      <label v-if="!personal">{{ t('calendar.calendarName') }}<input v-model="f.name" type="text" maxlength="80" :placeholder="t('calendar.calendarNamePlaceholder')" /></label>
      <div v-else class="calm-personal"><AppIcon name="user" /><b>{{ t('calendar.personal') }}</b><span class="hint">{{ t('calendar.personalHint') }}</span></div>
      <div class="ev-colors" role="radiogroup" :aria-label="t('calendar.color')">
        <button v-for="(c, k) in CAL_COLORS" :key="k" type="button" class="ev-color" :class="{ on: f.color === k }" :style="{ background: c.bg }"
                :title="colorLabel(k)" :aria-label="colorLabel(k)" role="radio" :aria-checked="f.color === k" @click="f.color = k as string"></button>
      </div>

      <template v-if="!isNew">
        <section class="calm-sec">
          <h3><AppIcon name="users" />{{ t('calendar.sharing') }}</h3>
          <p class="hint">{{ t('calendar.sharingHint') }}</p>
          <div v-for="(s, i) in shares" :key="s.userId" class="calm-share">
            <AppAvatar :login="s.login" :name="s.name || s.login" style="width:28px;height:28px;font-size:11px" />
            <span class="calm-who"><b>{{ s.name || s.login }}</b><span class="hint">{{ s.login }}</span></span>
            <span class="seg sm" role="group">
              <button type="button" :class="{ on: !s.canWrite }" @click="s.canWrite = false">{{ t('calendar.canRead') }}</button>
              <button type="button" :class="{ on: s.canWrite }" @click="s.canWrite = true">{{ t('calendar.canEdit') }}</button>
            </span>
            <button type="button" class="icon-btn" :aria-label="t('calendar.unshare', { name: s.name || s.login })" @click="shares.splice(i, 1)"><AppIcon name="close" /></button>
          </div>
          <div v-if="free.length" class="calm-add">
            <AppSelect v-model="addWho" :options="freeOptions" :aria-label="t('calendar.addPerson')" searchable />
            <button type="button" class="btn sm" :disabled="addWho === ''" @click="addShare"><AppIcon name="plus" />{{ t('calendar.share') }}</button>
          </div>
        </section>

        <section class="calm-sec">
          <h3><AppIcon name="link" />{{ t('calendar.icsTitle') }}</h3>
          <p class="hint">{{ t('calendar.icsHint') }}</p>
          <template v-if="icsUrl">
            <div class="calm-link"><code>{{ icsUrl }}</code><button type="button" class="btn sm" @click="copy(icsUrl)"><AppIcon name="copy" />{{ t('common.copy') }}</button></div>
            <div class="calm-row">
              <button type="button" class="link" @click="setIcs(true, true)">{{ t('calendar.icsRotate') }}</button>
              <button type="button" class="link danger-text" @click="setIcs(false)">{{ t('calendar.icsOff') }}</button>
            </div>
          </template>
          <button v-else type="button" class="btn sm" @click="setIcs(true)"><AppIcon name="share" />{{ t('calendar.icsOn') }}</button>
        </section>

        <section class="calm-sec">
          <h3><AppIcon name="phone" />{{ t('calendar.onPhone') }}</h3>
          <p class="hint">{{ t('calendar.onPhoneHint') }}</p>
          <button type="button" class="link" @click="openDevices">{{ t('calendar.connectPhone') }}</button>
        </section>
      </template>
      <p v-if="err" class="err" role="alert">{{ err }}</p>
    </form>
    <template #foot>
      <button v-if="!isNew && !personal" class="btn danger-text" type="button" @click="remove"><AppIcon name="trash" />{{ t('common.delete') }}</button>
      <span class="grow"></span>
      <button class="btn" type="button" @click="cal.calEditing = null">{{ t('common.cancel') }}</button>
      <button class="btn primary" type="button" :disabled="busy" @click="save">{{ isNew ? t('common.create') : t('common.save') }}</button>
    </template>
  </AppModal>
</template>

<style scoped>
.calm{gap:14px}
.calm-personal{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.calm-personal svg{width:18px;height:18px;color:var(--accent)}
.calm-personal .hint{flex-basis:100%;font-size:12.5px}
.calm-sec{display:flex;flex-direction:column;gap:8px;padding-top:12px;border-top:1px solid var(--line)}
.calm-sec h3{display:flex;align-items:center;gap:8px;margin:0;font-size:14px}
.calm-sec h3 svg{width:16px;height:16px;color:var(--accent)}
.calm-sec .hint{margin:0;font-size:12.5px}
.calm-share{display:flex;align-items:center;gap:10px}
.calm-who{display:flex;flex-direction:column;flex:1;min-width:0}
.calm-who .hint{font-size:12px}
.calm-add{display:flex;gap:8px;align-items:center}
.calm-add > :first-child{flex:1}
.calm-link{display:flex;gap:8px;align-items:center}
.calm-link code{flex:1;min-width:0;font-family:var(--f-mono);font-size:12.5px;padding:7px 10px;background:var(--surface-2);border:1px solid var(--line);border-radius:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;user-select:all}
.calm-row{display:flex;gap:16px}
.calm-sec > .link{align-self:flex-start}
@media (max-width:560px){.calm-share{flex-wrap:wrap}.calm-link{flex-direction:column;align-items:stretch}}
</style>
