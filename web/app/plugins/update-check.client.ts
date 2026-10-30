// TMAIL is a single-page app: a tab opened before an update keeps running the old code.
// Every few minutes compare the build id with the server's: a hidden tab takes the new version by
// itself, an open one is offered to reload.
import { toast } from '~/composables/toast'
import { mail } from '~/composables/mail'
import { t } from '~/composables/i18n'

// A letter being written (window or reply in the reader): never reload under it.
const mailBusy = () => !!mail.compose

export default defineNuxtPlugin((nuxtApp) => {
  const current = useRuntimeConfig().app.buildId
  // A part of the old version is gone from the server anyway: reload instead of a dead button.
  nuxtApp.hook('app:chunkError', () => { reloadNuxtApp({ persistState: false, ttl: 60000 }) })
  window.addEventListener('vite:preloadError', (e) => { e.preventDefault(); reloadNuxtApp({ persistState: false, ttl: 60000 }) })
  let offered = false
  async function check() {
    try {
      const r = await fetch('/_nuxt/builds/latest.json', { cache: 'no-store' })
      if (!r.ok) return
      const { id } = await r.json()
      if (id && current && id !== current) {
        // Nobody is looking at the tab, and nothing is being written: just take the new version.
        // The address keeps the folder and the open letter, so the tab comes back as it was.
        if (document.visibilityState !== 'visible' && !mailBusy()) { location.reload(); return }
        if (offered) return
        offered = true
        toast(t('app.updateAvailable'), { ms: 600000, action: { label: t('app.reloadPage'), run: () => location.reload() } })
      }
    } catch {}
  }
  setInterval(check, 5 * 60 * 1000)
  document.addEventListener('visibilitychange', check)
  setTimeout(check, 30 * 1000)
})
