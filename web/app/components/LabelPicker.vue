<script setup lang="ts">
// "Labels" menu: tick labels for the given letters (all of them get or lose the label).
import { computed } from 'vue'
import { act, type Summary } from '~/composables/mail'
import { labels, LABEL_COLORS } from '~/composables/labels'
import { t } from '~/composables/i18n'

const props = defineProps<{ items: Summary[] }>()
const state = (id: number) => {
  const n = props.items.filter(m => m.labels?.includes(id)).length
  return n === 0 ? 'none' : n === props.items.length ? 'all' : 'some'
}
function toggle(id: number) { act(state(id) === 'all' ? 'unlabel' : 'label', props.items, String(id)) }
const has = computed(() => labels.list.length > 0)
</script>

<template>
  <ToolPop v-slot="{ close }" :title="t('ui.labels')" icon="tag" :down="true">
    <div class="move-list label-pick">
      <label v-for="l in labels.list" :key="l.id" class="tp-item">
        <input type="checkbox" :checked="state(l.id) === 'all'" :indeterminate.prop="state(l.id) === 'some'" @change="toggle(l.id)" />
        <i class="lbl-dot" :style="{ background: LABEL_COLORS[l.color]?.bg }"></i>{{ l.name }}
      </label>
      <p v-if="!has" class="hint" style="padding:6px 10px">{{ t('ui.noLabels') }}</p>
      <button type="button" class="tp-item" style="color:var(--accent)" @click="close(); labels.edit = { id: 0, name: '', color: 'blue' }"><AppIcon name="plus" /> {{ t('ui.newLabel') }}</button>
    </div>
  </ToolPop>
</template>
