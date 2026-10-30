<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'
import { t } from '~/composables/i18n'
const props = withDefaults(defineProps<{ title: string, width?: string, busy?: string, top?: boolean }>(), { width: '640px' })
const emit = defineEmits<{ close: [] }>()
const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <!-- top: questions asked while the compose window is open (it stands above ordinary modals) -->
    <div class="scrim" :class="{ top: props.top }" @mousedown.self="emit('close')">
      <div class="modal veil-host" role="dialog" aria-modal="true" :aria-label="props.title" :style="{ width: `min(${props.width}, 100%)` }">
        <!-- busy: the text of what the modal is doing (loading, saving); veils it with the spinner -->
        <LoadingVeil :show="!!props.busy" :text="props.busy" />
        <div class="modal-head"><b>{{ props.title }}</b>
          <button class="icon-btn" :aria-label="t('common.close')" @click="emit('close')"><AppIcon name="close" /></button></div>
        <slot name="tabs" />
        <div class="modal-body"><slot /></div>
        <div v-if="$slots.foot" class="drawer-foot"><slot name="foot" /></div>
      </div>
    </div>
  </Teleport>
</template>
