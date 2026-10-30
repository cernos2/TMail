// Thin wrapper over $fetch: sends the anti-CSRF header, turns server errors into readable messages
// and sends the user to the login page when the session has expired.
import { t } from './i18n'
import { translateServer } from './servermsg'

export class ApiError extends Error {
  constructor(message: string, public status = 0) { super(message) }
}

// takePrefetched: an answer the page's head script already asked for (nuxt.config), used once.
function takePrefetched(path: string, opts: Record<string, any>): Promise<any> | undefined {
  const pre = (window as any).__zmPre as Record<string, { p: Promise<any>, t: number }> | undefined
  if (!pre || (opts.method && opts.method !== 'GET')) return
  const q = Object.entries(opts.query || {}).map(([k, v]) => `${k}=${v}`).join('&')
  const key = q ? `${path}?${q}` : path
  const hit = pre[key]
  if (!hit) return
  delete pre[key]
  return Date.now() - hit.t < 20000 ? hit.p : undefined
}

export async function api<T = any>(path: string, opts: Record<string, any> = {}): Promise<T> {
  const pre = takePrefetched(path, opts)
  if (pre) {
    try { return await pre } catch { /* asked again below, with the usual error handling */ }
  }
  try {
    return await $fetch<T>(path, {
      ...opts,
      credentials: 'same-origin',
      headers: { 'X-ZM': '1', ...(opts.headers || {}) },
    })
  } catch (e: any) {
    const status: number = e?.response?.status ?? 0
    if (status === 401 && !path.startsWith('/api/auth/')) {
      // Back to the same screen after logging in (a link from a colleague, a letter, a card).
      const here = useRouter().currentRoute.value
      await navigateTo(here.path === '/login' ? here.fullPath : { path: '/login', query: here.fullPath !== '/' ? { next: here.fullPath } : {} })
    }
    const msg = (typeof e?.data?.error === 'string' && e.data.error ? translateServer(e.data.error) : '')
      || (status ? t('mail.serverError', { status }) : t('mail.offline'))
    throw new ApiError(msg, status)
  }
}
