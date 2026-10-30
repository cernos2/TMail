<script setup lang="ts">
// Microphone button: first click starts recording, second click stops and returns the text.
import { computed } from 'vue'
import { voice, startVoice, stopVoice } from '~/composables/voice'
import { t } from '~/composables/i18n'

const props = defineProps<{ id: string, label?: string, small?: boolean }>()
const emit = defineEmits<{ text: [string] }>()
const mine = computed(() => voice.owner === props.id)
const rec = computed(() => voice.recording && mine.value)
const busy = computed(() => voice.busy && mine.value)
const time = computed(() => `${Math.floor(voice.seconds / 60)}:${String(voice.seconds % 60).padStart(2, '0')}`)

async function click() {
  if (busy.value) return
  if (rec.value) { const text = await stopVoice(); if (text) emit('text', text); return }
  await startVoice(props.id)
}
async function cancel() { await stopVoice(true) }
</script>

<template>
  <span class="voice" :class="{ rec, busy }">
    <button type="button" :class="props.small ? 'tb-btn' : 'btn'" :disabled="busy || (voice.recording && !mine)"
            :title="rec ? t('voice.stopAndRecognize') : t('voice.dictate')" :aria-label="rec ? t('voice.stopRecording') : t('voice.dictate')" @mousedown.prevent @click="click">
      <AppIcon :name="rec ? 'stop' : 'mic'" />
      <template v-if="!props.small || rec || busy">
        <span v-if="rec" class="v-dot"></span>
        <span>{{ busy ? t('voice.recognizing') : rec ? t('voice.doneTime', { time }) : props.label || t('voice.byVoice') }}</span>
      </template>
    </button>
    <button v-if="rec" type="button" class="tb-btn" :title="t('voice.cancelRecording')" :aria-label="t('voice.cancelRecording')" @click="cancel"><AppIcon name="close" /></button>
  </span>
</template>
