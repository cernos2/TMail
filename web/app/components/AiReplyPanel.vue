<script setup lang="ts">
// "Reply with AI": Loggi lists what the letter asks, the operator answers in a few words
// (or taps a suggested answer), ticks what to ask the sender — and Loggi writes the reply.
import { ref, onMounted } from 'vue'
import { replyPanel, replyInstructions, aiName } from '~/composables/ai'
import { t } from '~/composables/i18n'

const root = ref<HTMLElement | null>(null)
onMounted(() => root.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))

const emit = defineEmits<{ write: [instructions: string], close: [] }>()
const pick = (i: number, o: string) => { const q = replyPanel.questions[i]!; q.answer = q.answer === o ? '' : o }
const answered = () => replyPanel.questions.filter(q => q.answer.trim()).length
</script>

<template>
  <section ref="root" class="arp" :aria-label="t('ai.replyWithLabel', { ai: aiName() })">
    <header class="arp-head">
      <b><AppIcon name="sparkle" />{{ t('ai.replyWith', { ai: aiName() }) }}</b>
      <button class="icon-btn" :aria-label="t('common.close')" @click="emit('close')"><AppIcon name="close" /></button>
    </header>

    <div v-if="replyPanel.loading" class="arp-state veil-host t-loading"><LoadingVeil class="compact" show :text="t('ai.findingQuestions', { ai: aiName() })" /></div>
    <div v-else-if="replyPanel.error" class="arp-state err">{{ replyPanel.error }}</div>
    <template v-else>
      <p v-if="replyPanel.intent" class="arp-intent">{{ replyPanel.intent }}</p>

      <div v-if="replyPanel.questions.length" class="arp-block">
        <span class="arp-label">{{ t('ai.answerBriefly', { ai: aiName() }) }}</span>
        <div v-for="(q, i) in replyPanel.questions" :key="i" class="arp-q">
          <label :for="'arp-q' + i" class="arp-qtext">{{ q.question }}</label>
          <div v-if="q.options.length" class="arp-opts">
            <button v-for="o in q.options" :key="o" type="button" class="chip" :class="{ on: q.answer === o }" @click="pick(i, o)">{{ o }}</button>
          </div>
          <input :id="'arp-q' + i" v-model="q.answer" type="text" :placeholder="t('ai.ownAnswer')" />
        </div>
      </div>
      <p v-else class="arp-hint">{{ t('ai.noQuestions') }}</p>

      <div v-if="replyPanel.clarify.length" class="arp-block">
        <span class="arp-label">{{ t('ai.clarify') }}</span>
        <label v-for="(c, i) in replyPanel.clarify" :key="i" class="arp-check"><input v-model="c.on" type="checkbox" /> {{ c.text }}</label>
      </div>

      <div class="arp-block">
        <span class="arp-label">{{ t('ai.extra') }}</span>
        <div class="arp-extra">
          <textarea v-model="replyPanel.extra" rows="2" :placeholder="t('ai.extraPlaceholder')"></textarea>
          <VoiceButton id="arp" small @text="txt => replyPanel.extra = (replyPanel.extra ? replyPanel.extra + ' ' : '') + txt" />
        </div>
      </div>
    </template>

    <footer class="arp-foot">
      <button class="btn primary" :disabled="replyPanel.loading" @click="emit('write', replyInstructions())">
        <AppIcon name="sparkle" />{{ t('ai.writeReply') }}<span v-if="replyPanel.questions.length" class="arp-count">{{ answered() }}/{{ replyPanel.questions.length }}</span></button>
      <button class="btn" @click="emit('write', replyPanel.extra.trim())">{{ t('ai.writeNow') }}</button>
      <button class="link" @click="emit('close')">{{ t('common.cancel') }}</button>
    </footer>
  </section>
</template>

<style scoped>
.arp{border:1px solid color-mix(in srgb,var(--accent) 35%,var(--line));border-radius:7px;background:color-mix(in srgb,var(--accent) 4%,var(--surface));padding:14px 16px;display:flex;flex-direction:column;gap:12px;margin-top:12px}
.arp-head{display:flex;align-items:center;justify-content:space-between}
.arp-head b{display:inline-flex;align-items:center;gap:6px;color:var(--accent)}
.arp-head svg{width:16px;height:16px}
.arp-state{display:flex;align-items:center;gap:10px;color:var(--ink-2);font-size:14px;padding:6px 0}
.arp-state.err{color:var(--danger)}
.arp-intent{margin:0;font-size:14px;color:var(--ink-2)}
.arp-hint{margin:0;font-size:13px;color:var(--ink-3)}
.arp-block{display:flex;flex-direction:column;gap:8px}
.arp-label{font-size:12px;font-weight:600;color:var(--ink-3);text-transform:uppercase;letter-spacing:.04em}
.arp-q{display:flex;flex-direction:column;gap:6px;padding:10px 12px;border-radius:5px;background:var(--surface);border:1px solid var(--line)}
.arp-qtext{font-weight:600;font-size:14px}
.arp-opts{display:flex;flex-wrap:wrap;gap:6px}
.arp-q input,.arp-extra textarea{width:100%;border:1px solid var(--line);border-radius:4.5px;padding:7px 10px;background:var(--surface-2);color:var(--ink);font:inherit;font-size:14px}
.arp-extra{display:flex;gap:8px;align-items:flex-start}
.arp-extra textarea{resize:vertical;min-height:44px}
.arp-check{display:flex;align-items:center;gap:8px;font-size:14px}
.arp-foot{display:flex;flex-wrap:wrap;align-items:center;gap:8px}
.arp-count{margin-left:6px;font-size:12px;opacity:.8}
</style>
