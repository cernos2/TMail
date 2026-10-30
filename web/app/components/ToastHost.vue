<script setup lang="ts">
import { toasts, dismissToast, type Toast } from '~/composables/toast'

function run(t: Toast, f: () => void) { dismissToast(t.id); f() }
</script>

<template>
  <div class="toasts" role="status" aria-live="polite">
    <div v-for="t in toasts" :key="t.id" class="toast" :class="{ err: t.kind === 'err', clickable: !!t.click }"
         :role="t.click ? 'button' : undefined" :tabindex="t.click ? 0 : undefined"
         @click="t.click && run(t, t.click)" @keydown.enter="t.click && run(t, t.click)">
      <span>{{ t.text }}</span>
      <button v-if="t.action" class="link" @click.stop="run(t, t.action.run)">{{ t.action.label }}</button>
    </div>
  </div>
</template>
