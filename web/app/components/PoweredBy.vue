<script setup lang="ts">
// "TMAIL · open source" link to the project's repository (required by the license's attribution term).
import { REPO_URL } from '~/composables/brand'
import { t } from '~/composables/i18n'
</script>

<template>
  <p class="powered"><a :href="REPO_URL" target="_blank" rel="noopener">TMAIL</a> · {{ t('app.openSource') }}</p>
</template>

<style scoped>
.powered{margin:0;font-size:12px;color:var(--ink-3);text-align:center}
.powered a{color:inherit;font-weight:600;text-decoration:none}
.powered a:hover{color:var(--accent);text-decoration:underline}
</style>
