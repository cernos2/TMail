<script setup lang="ts">
// Formatting bar under the message body, like Gmail's "Aa" panel.
import { computed } from 'vue'
import { t } from '~/composables/i18n'

type Ed = { exec: (c: string, v?: string) => void, fmt: Record<string, boolean> }
const props = defineProps<{ ed: Ed | null }>()
const run = (c: string, v?: string) => props.ed?.exec(c, v)
const on = (c: string) => !!props.ed?.fmt[c]

const FONTS = computed(() => [
  { label: t('editor.fontSans'), v: 'Arial, Helvetica, sans-serif' }, { label: t('editor.fontSerif'), v: 'Georgia, "Times New Roman", serif' },
  { label: t('editor.fontMono'), v: '"Courier New", Courier, monospace' }, { label: t('editor.fontWide'), v: '"Arial Black", Arial, sans-serif' },
  { label: t('editor.fontNarrow'), v: '"Arial Narrow", Arial, sans-serif' }, { label: 'Tahoma', v: 'Tahoma, Verdana, sans-serif' },
  { label: 'Verdana', v: 'Verdana, Geneva, sans-serif' }, { label: 'Trebuchet MS', v: '"Trebuchet MS", sans-serif' },
])
const SIZES = computed(() => [{ label: t('editor.sizeSmall'), v: '1', px: 11 }, { label: t('editor.sizeNormal'), v: '3', px: 15 }, { label: t('editor.sizeLarge'), v: '5', px: 20 }, { label: t('editor.sizeHuge'), v: '7', px: 28 }])
const COLORS = ['#000000', '#444444', '#666666', '#999999', '#cccccc', '#eeeeee', '#ffffff',
  '#cc0000', '#e69138', '#f1c232', '#6aa84f', '#45818e', '#3d85c6', '#674ea7',
  '#ea9999', '#f9cb9c', '#ffe599', '#b6d7a8', '#a2c4c9', '#9fc5e8', '#b4a7d6',
  '#990000', '#b45f06', '#bf9000', '#38761d', '#134f5c', '#0b5394', '#351c75']
</script>

<template>
  <div class="fmt-bar" role="toolbar" :aria-label="t('editor.toolbar')" @mousedown.self.prevent>
    <button type="button" class="tb-btn" :title="t('editor.undoKey')" :aria-label="t('editor.undo')" @mousedown.prevent @click="run('undo')"><AppIcon name="undo" /></button>
    <button type="button" class="tb-btn" :title="t('editor.redoKey')" :aria-label="t('editor.redo')" @mousedown.prevent @click="run('redo')"><AppIcon name="redo" /></button>
    <i class="tb-sep"></i>
    <ToolPop v-slot="{ close }" :title="t('editor.font')" :label="t('editor.font')">
      <button v-for="f in FONTS" :key="f.v" type="button" class="tp-item" :style="{ fontFamily: f.v }" @mousedown.prevent @click="run('fontName', f.v); close()">{{ f.label }}</button>
    </ToolPop>
    <ToolPop v-slot="{ close }" :title="t('editor.size')" icon="textSize">
      <button v-for="s in SIZES" :key="s.v" type="button" class="tp-item" :style="{ fontSize: s.px + 'px' }" @mousedown.prevent @click="run('fontSize', s.v); close()">{{ s.label }}</button>
    </ToolPop>
    <i class="tb-sep"></i>
    <button type="button" class="tb-btn" :class="{ on: on('bold') }" :title="t('editor.boldKey')" :aria-label="t('editor.bold')" @mousedown.prevent @click="run('bold')"><AppIcon name="bold" /></button>
    <button type="button" class="tb-btn" :class="{ on: on('italic') }" :title="t('editor.italicKey')" :aria-label="t('editor.italic')" @mousedown.prevent @click="run('italic')"><AppIcon name="italic" /></button>
    <button type="button" class="tb-btn" :class="{ on: on('underline') }" :title="t('editor.underlineKey')" :aria-label="t('editor.underline')" @mousedown.prevent @click="run('underline')"><AppIcon name="underline" /></button>
    <ToolPop v-slot="{ close }" :title="t('editor.textColor')" icon="textColor">
      <div class="clr-cols">
        <div><span class="hint">{{ t('editor.background') }}</span>
          <div class="clr-grid"><button v-for="c in COLORS" :key="'b' + c" type="button" class="clr" :style="{ background: c }" :title="c" @mousedown.prevent @click="run('backColor', c); close()"></button></div>
        </div>
        <div><span class="hint">{{ t('editor.text') }}</span>
          <div class="clr-grid"><button v-for="c in COLORS" :key="'f' + c" type="button" class="clr" :style="{ background: c }" :title="c" @mousedown.prevent @click="run('foreColor', c); close()"></button></div>
        </div>
      </div>
    </ToolPop>
    <i class="tb-sep"></i>
    <ToolPop v-slot="{ close }" :title="t('editor.align')" :icon="on('justifyCenter') ? 'alignCenter' : on('justifyRight') ? 'alignRight' : 'alignLeft'">
      <div class="tp-row">
        <button type="button" class="tb-btn" :class="{ on: on('justifyLeft') }" :title="t('editor.alignLeft')" :aria-label="t('editor.alignLeft')" @mousedown.prevent @click="run('justifyLeft'); close()"><AppIcon name="alignLeft" /></button>
        <button type="button" class="tb-btn" :class="{ on: on('justifyCenter') }" :title="t('editor.alignCenter')" :aria-label="t('editor.alignCenter')" @mousedown.prevent @click="run('justifyCenter'); close()"><AppIcon name="alignCenter" /></button>
        <button type="button" class="tb-btn" :class="{ on: on('justifyRight') }" :title="t('editor.alignRight')" :aria-label="t('editor.alignRight')" @mousedown.prevent @click="run('justifyRight'); close()"><AppIcon name="alignRight" /></button>
      </div>
    </ToolPop>
    <button type="button" class="tb-btn" :class="{ on: on('insertOrderedList') }" :title="t('editor.numberedList')" :aria-label="t('editor.numberedList')" @mousedown.prevent @click="run('insertOrderedList')"><AppIcon name="listOl" /></button>
    <button type="button" class="tb-btn" :class="{ on: on('insertUnorderedList') }" :title="t('editor.bulletedList')" :aria-label="t('editor.bulletedList')" @mousedown.prevent @click="run('insertUnorderedList')"><AppIcon name="listUl" /></button>
    <button type="button" class="tb-btn" :title="t('editor.outdent')" :aria-label="t('editor.outdent')" @mousedown.prevent @click="run('outdent')"><AppIcon name="outdent" /></button>
    <button type="button" class="tb-btn" :title="t('editor.indent')" :aria-label="t('editor.indent')" @mousedown.prevent @click="run('indent')"><AppIcon name="indent" /></button>
    <button type="button" class="tb-btn" :title="t('editor.quote')" :aria-label="t('editor.quote')" @mousedown.prevent @click="run('formatBlock', 'blockquote')"><AppIcon name="quote" /></button>
    <button type="button" class="tb-btn" :class="{ on: on('strikeThrough') }" :title="t('editor.strike')" :aria-label="t('editor.strike')" @mousedown.prevent @click="run('strikeThrough')"><AppIcon name="strike" /></button>
    <button type="button" class="tb-btn" :title="t('editor.clearFormatting')" :aria-label="t('editor.clearFormatting')" @mousedown.prevent @click="run('removeFormat')"><AppIcon name="clearFmt" /></button>
  </div>
</template>
