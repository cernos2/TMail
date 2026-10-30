// Reactive "is the UI dark right now" flag: follows the explicit theme choice or the OS setting.
import { ref } from 'vue'

export const isDark = ref(false)
let started = false

export function watchTheme() {
  if (started || typeof window === 'undefined') return
  started = true
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const update = () => {
    const t = document.documentElement.getAttribute('data-theme')
    isDark.value = t ? t === 'dark' : mq.matches
  }
  mq.addEventListener('change', update)
  new MutationObserver(update).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  update()
}
