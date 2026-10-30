// Shared helpers for the README screenshots and videos (demo server only).
const puppeteer = require(process.env.PUPPETEER || 'puppeteer-core')
const os = require('os'), fs = require('fs')
const W = ms => new Promise(r => setTimeout(r, ms))
async function launch(port, tag, extraArgs = []) {
  const prof = `${os.homedir()}/snap/chromium/common/pp-readme-${tag}`
  fs.rmSync(prof, { recursive: true, force: true })
  return puppeteer.launch({ executablePath: '/snap/bin/chromium', headless: true, userDataDir: prof, protocolTimeout: 120000,
    args: ['--no-sandbox', `--host-resolver-rules=MAP mail.nordwave.example 127.0.0.1:${port}, MAP files.nordwave.example 127.0.0.1:${port}`, '--hide-scrollbars', '--font-render-hinting=none', ...extraArgs] })
}
async function setup(page, errs) {
  await page.evaluateOnNewDocument(() => { try { localStorage.setItem('zm-notify-banner', 'dismissed'); Object.defineProperty(Notification, 'permission', { get: () => 'granted' }); window.Notification = new Proxy(window.Notification, { construct: () => ({ close() {} }) }) } catch {} })
  page.on('pageerror', e => errs.push('PAGEERR ' + e.message))
  page.on('response', async r => { const u = r.url(); if (r.status() >= 400 && !/\/api\/(avatar|user-photo|auth\/|me$|ai\/actions)/.test(u) && !/\/api\/(settings|folders)$/.test(u)) errs.push('HTTP ' + r.status() + ' ' + u) })
}
const HOST = 'http://mail.nordwave.example'
module.exports = { W, launch, setup, HOST, fs, os }
