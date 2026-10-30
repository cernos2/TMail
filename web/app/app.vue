<script setup lang="ts">
import { onMounted } from 'vue'
import { applyAppearance, savedAppearance } from '~/composables/schemes'
import { watchTheme } from '~/composables/theme'
import { applyBackground, savedBackground } from '~/composables/backgrounds'

// Apply the remembered colours before the first API call to avoid a flash of the default scheme.
onMounted(() => {
  const s = savedAppearance()
  applyAppearance(s.scheme, s.theme)
  const bg = savedBackground()
  if (bg.id) applyBackground(bg.id, bg.version)
  watchTheme()
})
</script>

<template>
  <NuxtPage />
  <ToastHost />
  <DialogHost />
</template>
