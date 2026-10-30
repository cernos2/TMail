// Risky links in letters: a file to download from a file-sharing service or a link to a file that
// can carry a virus (a Word document with macros, an archive, a program), and links whose visible
// text names one site while they lead to another. Such links get a ⚠ mark in the letter and a
// warning before they are opened.
import { t } from './i18n'

const SHARING = ['dropbox.com', 'dropboxusercontent.com', 'disk.yandex.ru', 'disk.yandex.com', 'yadi.sk', 'disk.360.yandex.ru', 'drive.google.com',
  'docs.google.com', 'drive.usercontent.google.com', 'cloud.mail.ru', 'mega.nz', 'mega.co.nz', 'wetransfer.com', 'we.tl', '1drv.ms', 'onedrive.live.com',
  'sharepoint.com', 'mediafire.com', 'sendspace.com', 'files.fm', 'transfer.sh', 'gofile.io', 'anonfiles.com', 'pixeldrain.com', 'dosya.co',
  'disk.ru', 'filemail.com', 'icloud.com', 'box.com', 'pcloud.com', 'tusfiles.com', 'uploadfiles.io']
// Extensions that may hide a virus (macros, scripts, programs, archives with them).
const RISKY_EXT = /\.(docm?|xlsm?|xlsb|pptm?|rtf|zip|rar|7z|gz|tgz|ace|cab|iso|img|exe|scr|com|bat|cmd|msi|js|jse|vbs|vbe|wsf|hta|ps1|jar|lnk|apk)$/i

export type LinkRisk = { kind: 'download' | 'mismatch', host: string, shown?: string }

const hostOf = (u: URL) => u.hostname.toLowerCase().replace(/^www\./, '')
const under = (host: string, list: string[]) => list.some(d => host === d || host.endsWith('.' + d))

// A domain written in the link's text: «www.sberbank.ru», «https://bank.ru/login» (Cyrillic top-level domains too).
const reShownHost = /^\s*(?:https?:\/\/)?((?:[a-z0-9-]+\.)+[a-zа-я]{2,})(?:[/:?#]\S*)?\s*$/i

export function linkRisk(href: string, text: string, ownHosts: string[] = []): LinkRisk | null {
  let u: URL
  try { u = new URL(href) } catch { return null }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
  const host = hostOf(u)
  if (under(host, ownHosts)) return null
  // The text shows another site than the link goes to (a classic of phishing).
  const m = reShownHost.exec(text || '')
  if (m) {
    const shown = m[1]!.toLowerCase().replace(/^www\./, '')
    if (shown !== host && !host.endsWith('.' + shown) && !shown.endsWith('.' + host)) return { kind: 'mismatch', host, shown }
  }
  let path = u.pathname
  try { path = decodeURIComponent(path) } catch {}
  if (under(host, SHARING) || RISKY_EXT.test(path)) return { kind: 'download', host }
  return null
}

// Words for the warning.
export function riskText(r: LinkRisk, href: string) {
  let file = ''
  try { file = decodeURIComponent(new URL(href).pathname.split('/').pop() || '') } catch {}
  if (r.kind === 'mismatch') {
    return { title: t('mail.link.mismatchTitle'), text: t('mail.link.mismatchText', { shown: r.shown, host: r.host }) }
  }
  return {
    title: t('mail.link.downloadTitle'),
    text: file ? t('mail.link.downloadFileText', { host: r.host, file }) : t('mail.link.downloadText', { host: r.host }),
  }
}

// Our own large-file links (files.<domain>) are safe: checked by our antivirus.
export const ownHosts = () => [location.hostname.replace(/^mail\./, 'files.'), location.hostname]

// Risky links of a letter (for the warning above it).
export function riskyLinks(html: string): number {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const own = ownHosts()
  return Array.from(doc.querySelectorAll('a[href]')).filter(a => linkRisk(a.getAttribute('href') || '', a.textContent || '', own)).length
}
