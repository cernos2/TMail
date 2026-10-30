<script setup lang="ts">
// Renders already-sanitised message HTML in a sandboxed iframe: no scripts, its own CSP,
// remote images only when the user allows them. allow-same-origin (without allow-scripts)
// lets us measure the content height and recolour it for the dark theme.
import { computed, ref, onBeforeUnmount } from 'vue'
import { isDark } from '~/composables/theme'
import { darkenMail } from '~/composables/darkmail'
import { collapseQuotes } from '~/composables/quotes'
import { linkRisk, riskText, ownHosts } from '~/composables/links'
import { confirmDialog } from '~/composables/dialogs'
import { linkify, parseMailto } from '~/composables/linkify'
import { composeTo } from '~/composables/mail'
import { t } from '~/composables/i18n'

const props = defineProps<{ html: string, allowRemote: boolean, original?: boolean }>()
const frame = ref<HTMLIFrameElement | null>(null)
const height = ref(120)
let ro: ResizeObserver | null = null

const dark = computed(() => isDark.value && !props.original)
const BG_DARK = '#171E28'

const srcdoc = computed(() => {
  // Previews in our own large-file cards (files.<domain>) are always shown; other remote images on request.
  const own = `https://${location.hostname.replace(/^mail\./, 'files.')}`
  const img = props.allowRemote ? "'self' data: https: http:" : `'self' data: ${own}`
  const csp = `default-src 'none'; img-src ${img}; style-src 'unsafe-inline'; font-src data:${props.allowRemote ? ' https:' : ''}; media-src 'none'; form-action 'none'`
  const base = dark.value
    ? `html,body{background:${BG_DARK};color:#DCE3EA;color-scheme:dark} a{color:#8DB0FF} blockquote{border-color:#3A4656;color:#A3AFBC}`
    : `html,body{background:#fff;color:#1b232b} a{color:#0a4ff5}`
  return `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<base target="_blank">
<style>
html,body{margin:0;padding:0}
body{font:15px/1.6 Arial,Helvetica,sans-serif;word-wrap:break-word;overflow-wrap:anywhere}
img{max-width:100%;height:auto}
table{max-width:100%}
pre{white-space:pre-wrap}
blockquote{margin:0 0 0 .8ex;border-left:2px solid #ccc;padding-left:1ex;color:#555}
.tm-q{display:block;margin:10px 0;padding:0 8px;height:16px;line-height:10px;font:bold 14px/10px Arial,sans-serif;letter-spacing:1px;color:#5f6368;background:#e8eaed;border:0;border-radius:8px;cursor:pointer}
.tm-q:hover{background:#dadce0}
a.tm-risk::after{content:" ⚠";color:#d93025;text-decoration:none;font-weight:bold}
.tm-q[aria-expanded="true"]{background:#d2e3fc;color:#174ea6}
${base}
</style></head><body>${props.html}</body></html>`
})

// The content height. The frame is shrunk for the moment of measuring: a page's scrollHeight is
// never less than the frame itself, so measured as is the height could only grow (and stayed
// large after the earlier correspondence was folded behind «•••»).
function measure() {
  const f = frame.value
  const doc = f?.contentDocument
  if (!f || !doc?.documentElement) return
  const prev = f.style.height
  f.style.height = '0px'
  const h = doc.documentElement.scrollHeight
  f.style.height = prev
  height.value = Math.max(60, h)
}

function onLoad() {
  const doc = frame.value?.contentDocument
  if (doc && dark.value) darkenMail(doc)
  if (doc) collapseQuotes(doc, measure) // earlier correspondence behind «•••»
  if (doc) { linkify(doc); guardLinks(doc) }
  measure()
  ro?.disconnect()
  if (doc?.body && 'ResizeObserver' in window) {
    ro = new ResizeObserver(measure)
    ro.observe(doc.body)
  }
  // Images finishing later change the height too.
  doc?.querySelectorAll('img').forEach(img => img.addEventListener('load', measure, { once: true }))
}
onBeforeUnmount(() => ro?.disconnect())

// Risky links: a ⚠ mark, and a warning before opening. An e-mail address starts a new letter to it.
function guardLinks(doc: Document) {
  const own = ownHosts()
  doc.querySelectorAll('a[href]').forEach(a => {
    const href = a.getAttribute('href') || ''
    if (/^mailto:/i.test(href)) {
      a.setAttribute('title', t('frame.compose'))
      a.addEventListener('click', e => { e.preventDefault(); composeTo(parseMailto(href)) })
      return
    }
    const risk = linkRisk(href, a.textContent || '', own)
    if (!risk) return
    a.classList.add('tm-risk')
    a.setAttribute('title', risk.kind === 'mismatch' ? t('frame.riskMismatch', { host: risk.host }) : t('frame.riskDownload', { host: risk.host }))
    a.addEventListener('click', async (e) => {
      e.preventDefault()
      const w = riskText(risk, href)
      if (await confirmDialog(w.title, w.text, { ok: t('frame.openAnyway'), danger: true })) window.open(href, '_blank', 'noopener,noreferrer')
    })
  })
}
</script>

<template>
  <div class="frame-wrap" :class="{ dark }">
    <iframe ref="frame" :srcdoc="srcdoc" :style="{ height: height + 'px' }" :title="t('frame.title')"
            sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" @load="onLoad" />
  </div>
</template>
