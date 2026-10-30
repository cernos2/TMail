<script setup lang="ts">
// Attachment cards with a picture of the file, like Gmail: images scaled down, PDFs and office
// documents show their first page. Click opens the preview (or downloads what can't be previewed).
import { reactive, watch } from 'vue'
import type { Attachment } from '~/composables/mail'
import { attUrl, ext, extColor, canPreview, openAttachment, riskOf, RISK_TEXT, scanAttachment, scanOf, lockedText, type MsgRef } from '~/composables/attachments'
import { confirmDialog } from '~/composables/dialogs'
import { toast } from '~/composables/toast'
import { fileSize } from '~/composables/format'
import { t } from '~/composables/i18n'

const props = defineProps<{ msg: MsgRef, atts: Attachment[] }>()
const failed = reactive(new Set<string>())
const thumbable = (a: Attachment) => !!a.preview && a.preview !== 'text'
// Every attachment is checked by the antivirus as soon as the letter is open (one at a time),
// so "Download" is usually ready by the time it is clicked.
watch(() => props.atts.map(a => a.part).join(), async () => { for (const a of props.atts) await scanAttachment(props.msg, a) }, { immediate: true })
const scanLabel = (a: Attachment) => {
  const v = scanOf(props.msg, a)
  return !v || v.status === 'checking' ? t('attach.scanChecking') : v.status === 'clean' ? t('attach.scanClean') : v.status === 'virus' ? t('attach.scanVirus') : v.status === 'locked' ? (v.kind === 'encrypted' ? t('attach.scanEncrypted') : v.kind === 'heuristic' ? t('attach.scanSuspicious') : t('attach.scanUncheckable')) : t('attach.scanPending')
}
// "Download all": every file must be clean; a risky one asks once more.
async function zipAll(e: Event) {
  e.preventDefault()
  for (const a of props.atts) {
    let v = scanOf(props.msg, a)
    if (!v || v.status !== 'clean' && v.status !== 'virus') { toast(t('attach.scanningAll')); v = await scanAttachment(props.msg, a, true) }
    if (v.status === 'virus') { toast(t('attach.virusFound', { name: a.name, virus: v.virus }), { kind: 'err', ms: 9000 }); return }
    if (v.status === 'locked') { toast(lockedText(a, v), { kind: 'err', ms: 10000 }); return }
    if (v.status !== 'clean') { toast(t('attach.notScannedAll'), { kind: 'err', ms: 7000 }); return }
  }
  const bad = props.atts.find(a => riskOf(props.msg, a))
  if (bad) {
    const [title, text] = RISK_TEXT[riskOf(props.msg, bad) as keyof typeof RISK_TEXT]
    if (!await confirmDialog(t('attach.riskConfirm', { title, name: bad.name }), t('attach.riskZipText', { text }), { ok: t('attach.downloadAnyway'), danger: true })) return
  }
  location.href = zipUrl()
}
const zipUrl = () => `/api/attachments.zip?${new URLSearchParams({ account: props.msg.account, folder: props.msg.folder, uid: String(props.msg.uid) })}`
</script>

<template>
  <section class="atts-box" :aria-label="t('attach.attachments')">
  <div class="atts-head">
    <span class="atts-ico"><AppIcon name="clip" /></span>
    <b>{{ t('attach.count', { n: props.atts.length }) }}</b>
    <span class="atts-total">{{ fileSize(props.atts.reduce((n, a) => n + (a.size || 0), 0)) }}</span>
    <a v-if="props.atts.length > 1" class="btn sm" :href="zipUrl()" @click="zipAll"><AppIcon name="download" />{{ t('attach.downloadAll') }}</a>
  </div>
  <div class="atts">
    <button v-for="a in props.atts" :key="a.part" type="button" class="att" :title="canPreview(a) ? t('common.open') : t('common.download')" @click="openAttachment(props.msg, a, props.atts)">
      <div class="att-thumb" :class="{ pic: thumbable(a) && !failed.has(a.part) }">
        <img v-if="thumbable(a) && !failed.has(a.part)" :src="attUrl(props.msg, a, false, 'thumb')" :alt="a.name" loading="lazy" @error="failed.add(a.part)" />
        <AppIcon v-else name="file" />
        <span class="att-type" :style="{ background: extColor(a) }">{{ ext(a) || 'FILE' }}</span>
        <span v-if="riskOf(props.msg, a)" class="att-risk" :title="RISK_TEXT[riskOf(props.msg, a) as keyof typeof RISK_TEXT][1]"><AppIcon name="alert" /></span>
      </div>
      <div class="att-info"><span class="att-name" :title="a.name">{{ a.name }}</span><span class="att-meta">{{ fileSize(a.size) }} · {{ canPreview(a) ? t('attach.openLower') : t('attach.downloadLower') }}</span>
        <span class="att-av" :class="scanOf(props.msg, a)?.status"><AppIcon :name="scanOf(props.msg, a)?.status === 'virus' || scanOf(props.msg, a)?.status === 'locked' ? 'shieldX' : 'shield'" />{{ scanLabel(a) }}</span></div>
    </button>
  </div>
  </section>
</template>
