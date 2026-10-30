<script setup lang="ts">
// The one loading spinner of TMAIL: every wait looks the same (in a veil, a button, a viewer).
withDefaults(defineProps<{ size?: 'sm' | 'md' | 'lg', light?: boolean }>(), { size: 'md' })
</script>

<template>
  <span class="spinner" :class="[size, { light }]" aria-hidden="true"></span>
</template>
