<script setup lang="ts">
// Signature editor: formatting, links and pictures. A picture is uploaded to the file storage at
// once and the signature links to it (https://files.<domain>/i/…): letters do not carry it as an
// attachment. Old plain-text signatures open as text and are saved as HTML from then on.
import { ref, watch } from 'vue'
import { api } from '~/composables/api'
import { toast } from '~/composables/toast'
import { isHtmlSig, textHtml } from '~/composables/mail'
import { t } from '~/composables/i18n'

const props = defineProps<{ modelValue: string, placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const html = ref(isHtmlSig(props.modelValue) ? props.modelValue : textHtml(props.modelValue))
const ed = ref<any>(null)
const uploading = ref(0)
const fileInput = ref<HTMLInputElement | null>(null)
const linkOpen = ref(false)
const link = ref({ url: '', text: '' })

// The editor marks pictures for the letter's own attachments; a signature has none.
const clean = (h: string) => h.replace(/\sdata-cid="[^"]*"/g, '')
watch(html, v => emit('update:modelValue', clean(v)))
watch(() => props.modelValue, v => { if (v !== clean(html.value)) html.value = isHtmlSig(v) ? v : textHtml(v) })

const escRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
function addImage(f: File) {
  const url = URL.createObjectURL(f)
  upload(f, url)
  return { cid: '', url }
}
async function upload(f: File, blob: string) {
  uploading.value++
  try {
    const r = await api<{ url: string }>('/api/signature-image', { method: 'POST', body: f, headers: { 'Content-Type': f.type || 'application/octet-stream' } })
    html.value = html.value.split(blob).join(r.url)
  } catch (e: any) {
    html.value = html.value.replace(new RegExp(`<img[^>]*${escRe(blob)}[^>]*>`, 'g'), '')
    toast(e.message, { kind: 'err' })
  } finally {
    uploading.value--
    URL.revokeObjectURL(blob)
  }
}
function pickImages(e: Event) {
  const list = Array.from((e.target as HTMLInputElement).files || [])
  if (list.length) ed.value?.insertImageFiles(list)
  ;(e.target as HTMLInputElement).value = ''
}
function openLink() {
  link.value = { url: '', text: ed.value?.selectedText() || '' }
  linkOpen.value = !linkOpen.value
}
function insertLink() {
  if (!link.value.url.trim()) return
  ed.value?.insertLink(link.value.url.trim(), link.value.text.trim())
  linkOpen.value = false
}
defineExpose({ busy: () => uploading.value > 0 })
</script>

<template>
  <div class="sig-editor">
    <div class="sig-tools">
      <EditorToolbar :ed="ed" />
      <button type="button" class="tb-btn" :class="{ on: linkOpen }" :title="t('editor.insertLink')" :aria-label="t('editor.insertLink')" @mousedown.prevent @click="openLink"><AppIcon name="link" /></button>
      <button type="button" class="tb-btn" :title="t('editor.insertImageSig')" :aria-label="t('editor.insertImage')" @mousedown.prevent @click="fileInput?.click()"><AppIcon name="image" /></button>
      <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden @change="pickImages" />
    </div>
    <form v-if="linkOpen" class="sig-link" @submit.prevent="insertLink">
      <input v-model="link.url" type="text" :placeholder="t('editor.sigLinkPlaceholder')" :aria-label="t('editor.linkAddress')" autofocus />
      <input v-model="link.text" type="text" :placeholder="t('editor.linkTextOptional')" :aria-label="t('editor.linkText')" />
      <button class="btn sm primary" :disabled="!link.url.trim()">{{ t('editor.insert') }}</button>
      <button type="button" class="btn sm" @click="linkOpen = false">{{ t('common.cancel') }}</button>
    </form>
    <RichEditor ref="ed" v-model="html" class="sig-rte" :add-image="addImage" :placeholder="props.placeholder || t('editor.sigPlaceholder')" />
    <p v-if="uploading" class="hint sig-up"><AppSpinner size="sm" />{{ t('editor.uploadingImage') }}</p>
  </div>
</template>
