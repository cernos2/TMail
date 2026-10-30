<script setup lang="ts">
// Circle with a picture when one exists (our user's photo, the sender's brand logo, Gravatar),
// otherwise coloured initials. Pictures are proxied by the server, so senders can't track opens.
import { computed, ref, watch } from 'vue'
import { initials, avatarColor } from '~/composables/format'
import { avatars } from '~/composables/avatar'

const props = defineProps<{ email?: string, name?: string, login?: string, bimi?: boolean }>()
const loaded = ref(false)

const src = computed(() => {
  const v = avatars.version ? `&v=${avatars.version}` : ''
  if (props.login) return `/api/user-photo?login=${encodeURIComponent(props.login)}${v}`
  const e = (props.email || '').trim().toLowerCase()
  if (!e.includes('@')) return ''
  return `/api/avatar?email=${encodeURIComponent(e)}${props.bimi ? '&bimi=1' : ''}&src=2${v}` // src=2: site icons added, old misses not reused
})
const failed = computed(() => !src.value || avatars.misses.has(src.value))
watch(src, () => { loaded.value = false })
const label = computed(() => props.name || props.email || props.login || '')
</script>

<template>
  <div class="avatar" :style="{ background: loaded && !failed ? 'var(--surface-2)' : avatarColor(props.email || props.login || label) }" aria-hidden="true">
    <span v-if="!loaded || failed">{{ initials(label) }}</span>
    <img v-if="!failed" :src="src" alt="" loading="lazy" decoding="async" :class="{ on: loaded }"
         @load="loaded = true" @error="avatars.misses.add(src)" />
  </div>
</template>
