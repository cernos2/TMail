// Single-page app served as static files by the Go backend (zmaild).
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  ssr: false,
  devtools: { enabled: false },
  telemetry: false,
  css: [
    '@fontsource/onest/400.css',
    '@fontsource/onest/500.css',
    '@fontsource/onest/600.css',
    '@fontsource/onest/700.css',
    '@fontsource/unbounded/600.css',
    '@fontsource/unbounded/800.css',
    '@fontsource/jetbrains-mono/400.css',
    '@fontsource/jetbrains-mono/500.css',
    '~/assets/css/main.css',
  ],
  app: {
    head: {
      title: 'TMAIL',
      meta: [
        { name: 'viewport', content: 'width=device-width, initial-scale=1, viewport-fit=cover' },
        { name: 'theme-color', content: '#0A4FF5' },
        { name: 'robots', content: 'noindex, nofollow' },
      ],
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
        { rel: 'apple-touch-icon', href: '/icon-192.png' },
      ],
      // The data of the start screen is asked for while the scripts are still loading: the
      // account list, then each account's Inbox (see takePrefetched in api.ts). Plain "/" only.
      script: [{
        tagPosition: 'head',
        innerHTML: `(function(){try{if(location.pathname!=='/'||location.search)return;var P=window.__zmPre={};var g=function(u){var p=fetch(u,{credentials:'same-origin',headers:{'X-ZM':'1'}}).then(function(r){if(!r.ok)throw r;return r.json()});p.catch(function(){});P[decodeURIComponent(u)]={p:p,t:Date.now()};return p};g('/api/me').then(function(me){(me.accounts||[]).forEach(function(a){g('/api/messages?account='+encodeURIComponent(a.address)+'&limit=50&role=inbox')})}).catch(function(){});g('/api/settings');g('/api/folders')}catch(e){}})()`,
      }],
    },
  },
  devServer: { port: 3000 },
  // .br/.gz copies of the scripts and styles next to them: the server sends those (5–8× smaller).
  nitro: { compressPublicAssets: { gzip: true, brotli: true } },
})
