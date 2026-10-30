// User folders with a purpose: description for the AI, senders that always go there, auto-sorting switch.
import { reactive } from 'vue'
import { api } from './api'
import { loadFolders } from './mail'

export type FolderRule = { account: string, folder: string, description: string, senders: string[], auto: boolean, hideInbox?: boolean, expireDays?: number }
export type FolderEdit = { create: boolean, account: string, folder: string, name: string, description: string, senders: string, auto: boolean, hideInbox: boolean, expireDays: number }

export const folders = reactive({ rules: {} as Record<string, FolderRule[]>, edit: null as FolderEdit | null })

export async function loadRules(account: string) {
  try { folders.rules[account] = await api<FolderRule[]>('/api/folders/rules', { query: { account } }) } catch {}
}
export const ruleOf = (account: string, folder: string) => folders.rules[account]?.find(r => r.folder === folder)

export function newFolder(account: string) {
  folders.edit = { create: true, account, folder: '', name: '', description: '', senders: '', auto: true, hideInbox: false, expireDays: 0 }
}
export async function editFolder(account: string, folder: string) {
  if (!folders.rules[account]) await loadRules(account)
  const r = ruleOf(account, folder)
  folders.edit = { create: false, account, folder, name: folder, description: r?.description || '', senders: (r?.senders || []).join('\n'), auto: r?.auto ?? false, hideInbox: r?.hideInbox ?? false, expireDays: r?.expireDays ?? 0 }
}

export async function saveFolder(e: FolderEdit) {
  const body = { account: e.account, folder: e.create ? e.name.trim() : e.folder, rename: e.create ? '' : e.name.trim(),
    description: e.description, senders: [e.senders], auto: e.auto || e.hideInbox, hideInbox: e.hideInbox, expireDays: e.expireDays }
  await api('/api/folders', { method: e.create ? 'POST' : 'PUT', body })
  await Promise.all([loadRules(e.account), loadFolders()])
}
export async function deleteFolder(account: string, folder: string) {
  await api('/api/folders', { method: 'DELETE', query: { account, folder } })
  await Promise.all([loadRules(account), loadFolders()])
}

// "All letters from this sender go to this folder", from a letter: the sender joins the folder's
// rule, and the letters already in the Inbox move there on the server.
export async function addSenderToFolder(account: string, folder: string, sender: string) {
  await api('/api/folders/sender', { method: 'POST', body: { account, folder, sender } })
  await Promise.all([loadRules(account), loadFolders()])
}
// Big free mail services: a rule for the whole domain would catch everybody.
export const PUBLIC_DOMAINS = new Set(['gmail.com', 'googlemail.com', 'mail.ru', 'bk.ru', 'inbox.ru', 'list.ru', 'internet.ru', 'yandex.ru', 'ya.ru',
  'yandex.com', 'rambler.ru', 'outlook.com', 'hotmail.com', 'live.com', 'icloud.com', 'me.com', 'yahoo.com', 'proton.me', 'protonmail.com', 'aol.com', 'gmx.com', 'gmx.de', 'ukr.net', 'tut.by'])
