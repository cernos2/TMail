// Settings for mail programs (Outlook, Thunderbird, iPhone, Android): same server for every
// mailbox, the login is the full address, the password is the mailbox's own one.
import { t, i18n } from '~/composables/i18n'
import { mail } from '~/composables/mail'

export const MAIL_HOST = typeof window !== 'undefined' ? window.location.hostname : 'localhost'

export type ClientRow = { key: string, label: string, value: string, alt?: string, copy?: boolean }

export function clientSettings(address: string, password?: string): ClientRow[] {
  return [
    { key: 'login', label: t('mailclient.labelLogin'), value: address, copy: true },
    { key: 'pass', label: t('common.password'), value: password || t('mailclient.passwordFromAdmin'), copy: !!password },
    { key: 'imap', label: t('mailclient.labelImap'), value: t('mailclient.serverPort', { host: MAIL_HOST, port: 993 }), alt: t('mailclient.altPort', { port: 143 }), copy: false },
    { key: 'pop3', label: t('mailclient.labelPop3'), value: t('mailclient.serverPort', { host: MAIL_HOST, port: 995 }), copy: false },
    { key: 'smtp', label: t('mailclient.labelSmtp'), value: t('mailclient.serverPort', { host: MAIL_HOST, port: 465 }), alt: t('mailclient.altPort', { port: 587 }), copy: false },
    { key: 'auth', label: t('mailclient.labelAuth'), value: t('mailclient.authValue'), copy: false },
  ]
}

export function settingsText(address: string, name: string, password?: string) {
  return [name ? t('mailclient.textTitleNamed', { address, name }) : t('mailclient.textTitle', { address }), '',
    t('mailclient.textLogin', { address }), t('mailclient.textPassword', { password: password || t('mailclient.passwordFromAdminParen') }), '',
    t('mailclient.textImap', { host: MAIL_HOST, port: 993 }), t('mailclient.textPop3', { host: MAIL_HOST, port: 995 }),
    t('mailclient.textSmtp', { host: MAIL_HOST, port: 465, alt: 587 }), t('mailclient.textSmtpAuth'), '',
    t('mailclient.textWebmail', { url: `https://${MAIL_HOST}` })].join('\n')
}

const esc = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]!))
const uuid = () => (crypto as any).randomUUID ? crypto.randomUUID().toUpperCase() : `${Date.now()}-${Math.random()}`.toUpperCase()

// Apple configuration profile (iPhone, iPad, Mac): opening it adds the account in one tap.
// Without a password inside, the device asks for it while installing.
export function mobileconfig(address: string, name: string, password?: string) {
  const s = (k: string, v: string) => `<key>${k}</key><string>${esc(v)}</string>`
  const i = (k: string, v: number) => `<key>${k}</key><integer>${v}</integer>`
  const b = (k: string, v: boolean) => `<key>${k}</key><${v}/>`
  const id = `com.tmail.${address.replace(/[^a-z0-9]/gi, '.')}`
  const mail = [
    s('PayloadType', 'com.apple.mail.managed'), s('PayloadIdentifier', id + '.mail'), s('PayloadUUID', uuid()), i('PayloadVersion', 1),
    s('PayloadDisplayName', t('mailclient.profileName', { address })), s('EmailAccountType', 'EmailTypeIMAP'), s('EmailAccountDescription', `TMAIL · ${address}`),
    s('EmailAccountName', name || address), s('EmailAddress', address),
    s('IncomingMailServerHostName', MAIL_HOST), i('IncomingMailServerPortNumber', 993), b('IncomingMailServerUseSSL', true),
    s('IncomingMailServerAuthentication', 'EmailAuthPassword'), s('IncomingMailServerUsername', address),
    s('OutgoingMailServerHostName', MAIL_HOST), i('OutgoingMailServerPortNumber', 465), b('OutgoingMailServerUseSSL', true),
    s('OutgoingMailServerAuthentication', 'EmailAuthPassword'), s('OutgoingMailServerUsername', address), b('OutgoingPasswordSameAsIncomingPassword', true),
    ...(password ? [s('IncomingPassword', password), s('OutgoingPassword', password)] : []),
  ].join('')
  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadContent</key><array><dict>${mail}</dict></array>
${s('PayloadType', 'Configuration')}${s('PayloadIdentifier', id)}${s('PayloadUUID', uuid())}${i('PayloadVersion', 1)}
${s('PayloadDisplayName', t('mailclient.profileName', { address }))}${s('PayloadDescription', t('mailclient.profileDescription'))}${s('PayloadOrganization', mail.me?.defaults?.org || 'TMAIL')}
</dict></plist>`
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([xml], { type: 'application/x-apple-aspen-config' }))
  a.download = `${address}.mobileconfig`
  a.click()
  setTimeout(() => URL.revokeObjectURL(a.href), 5000)
}

// Printable card (or "Save as PDF" in the print dialog).
export function printSettings(cards: { address: string, name: string, password?: string }[]) {
  const card = (c: { address: string, name: string, password?: string }) => `
    <section><h2>${esc(c.address)}</h2>${c.name ? `<p class="n">${esc(c.name)}</p>` : ''}
    <table>${clientSettings(c.address, c.password).map(r => `<tr><th>${esc(r.label)}</th><td>${esc(r.value)}${r.alt ? `<br><small>${esc(r.alt)}</small>` : ''}</td></tr>`).join('')}</table></section>`
  const w = window.open('', '_blank')
  if (!w) return
  w.document.write(`<!doctype html><html lang="${esc(i18n.lang)}"><head><meta charset="utf-8"><title>${esc(t('mailclient.printDocTitle'))}</title><style>
    body{font:14px/1.45 Arial,sans-serif;color:#111;margin:24px}h1{font-size:18px;margin:0 0 4px}.sub{color:#555;margin:0 0 18px}
    section{border:1px solid #ccc;border-radius:10px;padding:14px 16px;margin:0 0 14px;break-inside:avoid}h2{font-size:16px;margin:0}.n{margin:2px 0 8px;color:#555}
    table{border-collapse:collapse;width:100%}th{text-align:left;font-weight:normal;color:#555;width:42%;padding:5px 8px 5px 0;vertical-align:top}td{padding:5px 0;font-weight:bold}
    small{font-weight:normal;color:#555}.foot{color:#555;font-size:12px;margin-top:14px}</style></head><body>
    <h1>${esc(t('mailclient.printHeading'))}</h1><p class="sub">${esc(t('mailclient.printSub', { url: `https://${MAIL_HOST}` }))}</p>
    ${cards.map(card).join('')}
    <p class="foot">${esc(t('mailclient.printFoot'))}</p>
    <script>setTimeout(()=>print(),200)</script></body></html>`)
  w.document.close()
}
