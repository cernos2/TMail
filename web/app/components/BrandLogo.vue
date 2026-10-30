<script setup lang="ts">
// TMAIL logo: the folded-ribbon T mark (public/logo-mark.svg) plus the wordmark.
const props = withDefaults(defineProps<{ text?: boolean, size?: 'sm' | 'md' | 'lg' }>(), { text: true, size: 'md' })
</script>

<template>
  <span class="brand" :class="'brand-' + props.size">
    <img src="/logo-mark.svg" alt="" class="brand-img" />
    <span v-if="props.text" class="brand-word">TMAIL</span>
    <span v-else class="sr-only">TMAIL</span>
  </span>
</template>
