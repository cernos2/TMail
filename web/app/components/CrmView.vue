<script setup lang="ts">
// Counterparties: list of companies and people on the left, the open card on the right.
import { onMounted, ref } from 'vue'
import { mail } from '~/composables/mail'
import { crm, loadCrmList, openParty, emptyParty, taxIdLabel, cardCountry } from '~/composables/crm'
import { shortTime } from '~/composables/format'
import { t } from '~/composables/i18n'

let timer: ReturnType<typeof setTimeout> | null = null
function onSearch() { if (timer) clearTimeout(timer); timer = setTimeout(loadCrmList, 250) }
function setKind(k: '' | 'company' | 'person') { crm.kind = k; loadCrmList() }
function create(kind: 'company' | 'person') { crm.current = null; crm.editing = emptyParty(kind) }
const newOpen = ref(false)
onMounted(loadCrmList)
</script>

<template>
  <div class="crm">
    <aside class="crm-list" :class="{ hidden: crm.current || crm.editing }">
      <header class="crm-head">
        <button class="icon-btn back-btn" :aria-label="t('crm.backToMail')" @click="mail.reading = false"><AppIcon name="back" /></button>
        <h2>{{ t('crm.title') }}</h2>
        <span class="grow"></span>
        <ToolPop v-slot="{ close }" :title="t('crm.newCard')" icon="plus" :down="true" align="right">
          <button class="tp-item" @click="close(); create('company')">{{ t('crm.company') }}</button>
          <button class="tp-item" @click="close(); create('person')">{{ t('crm.individual') }}</button>
        </ToolPop>
      </header>
      <div class="crm-search">
        <input v-model="crm.q" type="search" :placeholder="t('crm.searchPlaceholder')" :aria-label="t('crm.searchAria')" @input="onSearch" />
        <span class="seg">
          <button :class="{ on: !crm.kind }" @click="setKind('')">{{ t('common.all') }}</button>
          <button :class="{ on: crm.kind === 'company' }" @click="setKind('company')">{{ t('crm.companies') }}</button>
          <button :class="{ on: crm.kind === 'person' }" @click="setKind('person')">{{ t('crm.people') }}</button>
        </span>
      </div>
      <div class="crm-items-wrap veil-host">
      <LoadingVeil :show="crm.loading" :text="crm.q ? t('crm.searching') : t('crm.loadingList')" />
      <div class="crm-items">
        <button v-for="p in crm.list" :key="p.id" type="button" class="crm-item" :class="{ on: crm.current?.id === p.id }" @click="openParty(p.id)">
          <span class="crm-ico" :class="p.kind"><AppIcon :name="p.kind === 'company' ? 'home' : 'user'" /></span>
          <span class="crm-txt">
            <b>{{ p.name }}<span v-if="p.own" class="crm-own">{{ t('crm.ownTag') }}</span><i v-if="p.pending" class="crm-dot" :title="t('crm.pendingTitle')"></i></b>
            <span>{{ p.kind === 'company' ? (p.inn ? `${taxIdLabel(cardCountry(p))} ${p.inn}` : p.domains[0] || p.emails[0] || '') : [p.position, p.companyName].filter(Boolean).join(' · ') || p.emails[0] }}</span>
          </span>
          <span class="crm-meta">{{ p.lastLetter ? shortTime(p.lastLetter) : '' }}</span>
        </button>
        <p v-if="!crm.loading && !crm.list.length" class="hint crm-empty">
          {{ crm.q ? t('crm.nothingFound') : t('crm.emptyList') }}</p>
      </div>
      </div>
    </aside>
    <section class="crm-card veil-host" :class="{ empty: !crm.current && !crm.editing && !crm.currentLoading }">
      <LoadingVeil :show="crm.currentLoading || !!crm.busy" :text="crm.busy || t('crm.loadingCard')" />
      <div class="crm-card-scroll">
        <CrmCard v-if="crm.current || crm.editing || crm.currentLoading" />
        <div v-else class="crm-placeholder hint">{{ t('crm.selectHint') }}</div>
      </div>
    </section>
  </div>
</template>
