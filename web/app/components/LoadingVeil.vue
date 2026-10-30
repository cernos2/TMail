<script setup lang="ts">
// A veil over the block whose data is loading, with the spinner in its middle. The block must be
// positioned (the "veil-host" class). It appears after a moment, so instant answers don't flash,
// and says so when the server is slow.
import { t } from '~/composables/i18n'
const props = defineProps<{ show: boolean, text?: string, slowText?: string, dark?: boolean }>()

const visible = ref(false)
const slow = ref(false)
let tShow: ReturnType<typeof setTimeout> | undefined
let tSlow: ReturnType<typeof setTimeout> | undefined
watch(() => props.show, (on) => {
  clearTimeout(tShow)
  clearTimeout(tSlow)
  slow.value = false
  if (!on) { visible.value = false; return }
  tShow = setTimeout(() => { visible.value = true }, 150)
  tSlow = setTimeout(() => { slow.value = true }, 3000)
}, { immediate: true })
onBeforeUnmount(() => { clearTimeout(tShow); clearTimeout(tSlow) })
</script>

<template>
  <Transition name="veil">
    <div v-if="visible" class="veil" :class="{ dark }" role="status" aria-live="polite" @click.stop>
      <div class="veil-box">
        <AppSpinner size="lg" :light="dark" />
        <span>{{ slow ? (slowText || t('ui.slow')) : (text || t('common.loading')) }}</span>
      </div>
    </div>
  </Transition>
</template>
