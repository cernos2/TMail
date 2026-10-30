// Labels (as in Gmail): coloured tags; a letter can have several. Stored as IMAP keywords.
import { reactive } from 'vue'
import { api } from './api'
import { CAL_COLORS } from './calendar'

export type Label = { id: number, name: string, color: string }
export const labels = reactive({ list: [] as Label[], edit: null as null | { id: number, name: string, color: string } })
export const LABEL_COLORS = CAL_COLORS

export async function loadLabels() { try { labels.list = await api<Label[]>('/api/labels') } catch {} }
export const labelById = (id: number) => labels.list.find(l => l.id === id)
export async function saveLabel(l: { id: number, name: string, color: string }) {
  const saved = l.id ? await api<Label>(`/api/labels/${l.id}`, { method: 'PUT', body: l }) : await api<Label>('/api/labels', { method: 'POST', body: l })
  await loadLabels()
  return saved
}
export async function deleteLabel(id: number) { await api(`/api/labels/${id}`, { method: 'DELETE' }); await loadLabels() }
