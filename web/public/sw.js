// TMAIL service worker: the app opens without a network (the shell is cached), and the folders,
// lists and letters looked at recently are shown from a copy until the connection is back.
// Mail data is kept only in this browser and is wiped on sign-out (message "logout").
const SHELL = 'tmail-shell-v2'
const DATA = 'tmail-data-v1'
// What is shown offline: the start data and letters opened recently.
const DATA_PATHS = ['/api/me', '/api/settings', '/api/folders', '/api/messages', '/api/message', '/api/thread', '/api/labels',
  '/api/identities', '/api/templates', '/api/calendar/events', '/api/calendar/plan', '/api/calendars', '/api/crm/parties', '/api/quota', '/api/scheduled']
const MAX_DATA = 400 // responses kept; the oldest go first

self.addEventListener('install', e => {
  e.waitUntil(caches.open(SHELL).then(c => c.addAll(['/', '/manifest.webmanifest', '/icon-192.png', '/favicon.svg', '/logo-mark.svg'])).then(() => self.skipWaiting()))
})

self.addEventListener('activate', e => {
  e.waitUntil(caches.keys().then(keys => Promise.all(keys.filter(k => k !== SHELL && k !== DATA).map(k => caches.delete(k)))).then(() => self.clients.claim()))
})

self.addEventListener('message', e => {
  if (e.data === 'logout') e.waitUntil(caches.delete(DATA))
})

async function trim() {
  const c = await caches.open(DATA)
  const keys = await c.keys()
  for (let i = 0; i < keys.length - MAX_DATA; i++) await c.delete(keys[i])
}

// Network first; a copy when offline (marked so the page can say it is a copy).
async function networkFirst(req, cacheName, keep) {
  try {
    const res = await fetch(req)
    if (res.ok && keep) {
      const copy = res.clone()
      caches.open(cacheName).then(c => c.put(req, copy)).then(cacheName === DATA ? trim : undefined).catch(() => {})
    }
    return res
  } catch (err) {
    const hit = await caches.match(req, { cacheName })
    if (!hit) throw err
    if (cacheName !== DATA) return hit
    const h = new Headers(hit.headers)
    h.set('X-TMail-Offline', '1')
    return new Response(hit.body, { status: hit.status, statusText: hit.statusText, headers: h })
  }
}

self.addEventListener('fetch', e => {
  const req = e.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.origin !== location.origin) return
  // Pages: the app shell (one page for every address).
  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).then(res => {
      if (res.ok && url.pathname === '/') caches.open(SHELL).then(c => c.put('/', res.clone())).catch(() => {})
      return res
    }).catch(() => caches.match('/', { cacheName: SHELL })))
    return
  }
  // Scripts and styles never change under the same name.
  if (url.pathname.startsWith('/_nuxt/') && !url.pathname.startsWith('/_nuxt/builds/')) {
    e.respondWith(caches.match(req, { cacheName: SHELL }).then(hit => hit || fetch(req).then(res => {
      if (res.ok) { const copy = res.clone(); caches.open(SHELL).then(c => c.put(req, copy)).catch(() => {}) }
      return res
    })))
    return
  }
  // Pictures and fonts of the app itself: from the cache, refreshed in the background.
  if (/\.(svg|png|webp|woff2?|ico)$/.test(url.pathname) && !url.pathname.startsWith('/api/')) {
    e.respondWith(caches.match(req, { cacheName: SHELL }).then(hit => {
      const net = fetch(req).then(res => { if (res.ok) { const copy = res.clone(); caches.open(SHELL).then(c => c.put(req, copy)).catch(() => {}) } return res })
      if (hit) { net.catch(() => {}); return hit }
      return net
    }))
    return
  }
  if (DATA_PATHS.some(p => url.pathname === p || url.pathname.startsWith(p + '/'))) {
    e.respondWith(networkFirst(req, DATA, true))
  }
})
