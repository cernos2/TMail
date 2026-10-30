// Reply templates: shared company ones (admins edit) and personal ones.
import { reactive } from 'vue'
import { api } from './api'

export type Template = { id: number, title: string, body: string, mailbox: string, shared: boolean, mine: boolean }
export const tpl = reactive({ list: [] as Template[], loaded: false })

export async function loadTemplates() {
  try { tpl.list = await api<Template[]>('/api/templates') } catch {}
  tpl.loaded = true
}
export async function saveTemplate(t: Partial<Template>) {
  if (t.id) await api(`/api/templates/${t.id}`, { method: 'PUT', body: t })
  else await api('/api/templates', { method: 'POST', body: t })
  await loadTemplates()
}
export async function deleteTemplate(id: number) {
  await api(`/api/templates/${id}`, { method: 'DELETE' })
  await loadTemplates()
}
export const templatesFor = (mailbox: string) => tpl.list.filter(t => !t.mailbox || t.mailbox === mailbox)
