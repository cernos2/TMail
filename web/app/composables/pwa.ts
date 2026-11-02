// Installing TMAIL as an app (Chrome, Edge, Android; Safari: "Add to Home Screen") and its
// offline mode, served by public/sw.js.
import { reactive } from 'vue'

export const pwa = reactive({
  installable: false, // the browser offered installing (Chrome, Edge, Android)
  installed: typeof window !== 'undefined' && (window.matchMedia?.('(display-mode: standalone)').matches || (navigator as any).standalone === true),
  ios: typeof navigator !== 'undefined' && /iphone|ipad|ipod/i.test(navigator.userAgent),
  offlineCopy: false, // the last data came from the offline copy
})

let deferred: any = null

export function initPWA() {
  if (typeof window === 'undefined' || !('serviceWorker' in navigator) || location.protocol !== 'https:') return
  navigator.serviceWorker.register('/sw.js').catch(() => {})
  window.addEventListener('beforeinstallprompt', (e: any) => { e.preventDefault(); deferred = e; pwa.installable = true })
  window.addEventListener('appinstalled', () => { pwa.installed = true; pwa.installable = false; deferred = null })
}

export async function installApp() {
  if (!deferred) return false
  deferred.prompt()
  const r = await deferred.userChoice.catch(() => null)
  deferred = null
  pwa.installable = false
  return r?.outcome === 'accepted'
}
