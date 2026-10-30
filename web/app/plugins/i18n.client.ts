// The language remembered on this device (or else the server's) is loaded before the first screen
// is drawn; /api/me may switch it afterwards (applyUserLocale).
import { setLang, setTimezone, savedLang, savedTz, browserLang } from '~/composables/i18n'

export default defineNuxtPlugin({
  name: 'i18n',
  enforce: 'pre',
  async setup() {
    setTimezone(savedTz())
    // Never chosen on this device (the sign-in page of a new user): the server's language, so the
    // page speaks the language the mail will speak after signing in.
    let lang = savedLang()
    if (!lang) {
      try { lang = (await (await fetch('/api/locale', { credentials: 'same-origin' })).json()).lang } catch {}
    }
    await setLang(lang || browserLang())
  },
})
