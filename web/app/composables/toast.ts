import { reactive } from 'vue'

// click: the whole notice is a link (e.g. "Message sent" opens the letter in Sent).
// keep: not pushed out by newer notices while it is up (a countdown with "Undo").
export type Toast = { id: number, text: string, kind: 'info' | 'err', action?: { label: string, run: () => void }, click?: () => void, keep?: boolean }
export const toasts = reactive<Toast[]>([])
let seq = 0

export function toast(text: string, opts: { kind?: 'info' | 'err', action?: Toast['action'], click?: () => void, keep?: boolean, ms?: number } = {}): Toast {
  const t: Toast = { id: ++seq, text, kind: opts.kind ?? 'info', action: opts.action, click: opts.click, keep: opts.keep }
  toasts.push(t)
  if (toasts.length > 3) {
    const i = toasts.findIndex(x => !x.keep)
    if (i >= 0) toasts.splice(i, 1)
  }
  setTimeout(() => dismissToast(t.id), opts.ms ?? (opts.kind === 'err' ? 6000 : 3500))
  return toasts.find(x => x.id === t.id) || t // the reactive copy: changing its text updates the screen
}

export function dismissToast(id: number) {
  const i = toasts.findIndex(x => x.id === id)
  if (i >= 0) toasts.splice(i, 1)
}
