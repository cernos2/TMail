// In-app confirmation and one-time secret dialogs (rendered by DialogHost).
import { reactive } from 'vue'
import { t } from './i18n'

export const confirmState = reactive({
  open: false, title: '', text: '', ok: '', danger: false,
  resolve: null as null | ((v: boolean) => void),
})

export function confirmDialog(title: string, text = '', opts: { ok?: string, danger?: boolean } = {}): Promise<boolean> {
  return new Promise(resolve => {
    Object.assign(confirmState, { open: true, title, text, ok: opts.ok || t('mail.confirm'), danger: !!opts.danger, resolve })
  })
}

export function closeConfirm(v: boolean) {
  confirmState.resolve?.(v)
  Object.assign(confirmState, { open: false, resolve: null })
}

export type SecretInfo = { title: string, secret: string, note?: string, rows?: [string, string][] }
export const secretState = reactive({ open: false, info: null as SecretInfo | null })
export function showSecret(info: SecretInfo) { Object.assign(secretState, { open: true, info }) }
export function closeSecret() { Object.assign(secretState, { open: false, info: null }) }

export async function copyText(s: string): Promise<boolean> {
  try { await navigator.clipboard.writeText(s); return true } catch { return false }
}
