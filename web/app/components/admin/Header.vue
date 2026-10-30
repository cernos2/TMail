<script setup lang="ts">
import { inject } from 'vue'
import { t } from '~/composables/i18n'
const props = defineProps<{ title: string, desc?: string }>()
const openMenu = inject<() => void>('openAdminMenu', () => {})
</script>

<template>
  <div class="adm-head">
    <button class="icon-btn menu-btn" :aria-label="t('admin.openMenu')" @click="openMenu()"><AppIcon name="menu" /></button>
    <h1>{{ props.title }}</h1>
    <div class="adm-tools"><slot /></div>
    <p v-if="props.desc">{{ props.desc }}</p>
  </div>
</template>
