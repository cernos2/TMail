// OpenPGP in the composer: whether the sender has a key and which recipients have none.
import { reactive } from 'vue'
import { api } from './api'

export type PgpCheck = { fromKey: boolean, signDefault: boolean, missing: string[] }
const cache = reactive(new Map<string, PgpCheck>())

// Addresses out of "Name <a@b>, c@d" fields.
export const addrs = (...fields: string[]) => fields.join(',').split(',').map(x => (x.match(/<([^>]+)>/)?.[1] || x).trim().toLowerCase()).filter(x => x.includes('@'))

export async function pgpCheck(from: string, rcpts: string[]): Promise<PgpCheck> {
  const k = `${from}|${rcpts.sort().join(',')}`
  const hit = cache.get(k)
  if (hit) return hit
  const r = await api<PgpCheck>('/api/pgp/status', { query: { from, to: rcpts.join(',') } })
  cache.set(k, r)
  return r
}
export function forgetPgpChecks() { cache.clear() }
