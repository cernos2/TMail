<script setup lang="ts">
// An earlier letter of the open conversation: a one-line summary that expands into the full letter.
import { computed } from 'vue'
import { mail, keyOf, expandInThread, imagesAllowed, type Summary } from '~/composables/mail'
import { longDate, displayName } from '~/composables/format'
import { ext, extColor, canPreview, openAttachment } from '~/composables/attachments'
import { t } from '~/composables/i18n'

const props = defineProps<{ m: Summary }>()
const full = computed(() => mail.threadOpen[keyOf(props.m)])
const msg = computed(() => typeof full.value === 'object' ? full.value : null)
const mine = computed(() => props.m.from.email === props.m.account)
const toLine = computed(() => props.m.to.map(a => a.email === props.m.account ? t('thread.me') : (a.name || a.email)).join(', '))
</script>

<template>
  <article class="t-item" :class="{ open: !!msg }" :data-tkey="keyOf(props.m)">
    <header class="t-head" role="button" tabindex="0" :aria-expanded="!!msg" @click="expandInThread(props.m)" @keydown.enter="expandInThread(props.m)">
      <AppAvatar :email="props.m.from.email" :name="displayName(props.m.from)" :bimi="props.m.dmarc === 'pass'" />
      <div class="t-who">
        <b>{{ mine ? t('thread.you', { name: displayName(props.m.from) }) : displayName(props.m.from) }}</b>
        <span v-if="msg" class="t-to">{{ t('thread.to', { to: toLine }) }}</span>
        <span v-else class="t-snip">{{ props.m.preview }}</span>
      </div>
      <span class="t-meta">
        <AppIcon v-if="props.m.hasAttachments" name="clip" />
        <span class="t-date">{{ longDate(props.m.date) }}</span>
      </span>
    </header>
    <div v-if="!msg && props.m.attachments?.length" class="t-files">
      <button v-for="a in props.m.attachments.slice(0, 4)" :key="a.part" type="button" class="file-pill"
              :title="canPreview(a) ? t('thread.filePreview', { name: a.name }) : t('thread.fileDownload', { name: a.name })" @click="openAttachment(props.m, a, props.m.attachments)">
        <span class="fp-ext" :style="{ background: extColor(a) }">{{ ext(a) || 'FILE' }}</span><span class="fp-name">{{ a.name }}</span>
      </button>
      <button v-if="props.m.attachments.length > 4" type="button" class="file-pill more" @click="expandInThread(props.m)">+{{ props.m.attachments.length - 4 }}</button>
    </div>
    <div v-if="full === 'loading'" class="t-body veil-host t-loading"><LoadingVeil class="compact" show :text="t('thread.loading')" /></div>
    <div v-else-if="full === 'error'" class="t-body hint">{{ t('thread.loadFailed') }}</div>
    <div v-else-if="msg" class="t-body">
      <MessageFrame :html="msg.html" :allow-remote="imagesAllowed(msg)" :original="mail.originalColors" />
      <AttachmentCards v-if="msg.attachments.length" :msg="msg" :atts="msg.attachments" />
    </div>
  </article>
</template>
