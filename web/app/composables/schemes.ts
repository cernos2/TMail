// Ready-made colour schemes; "TMAIL" is the brand default. Each defines light and dark variants.
import { t } from '~/composables/i18n'

// Names are getters so they follow the interface language.
export const SCHEMES = [
  { id: 'tmail', name: 'TMAIL', l: { accent: '#0A4FF5', ink: '#FFFFFF', soft: '#E3ECFF', line: '#B3C8FB', bg: '#EEF1F6', s2: '#F4F6FA', s3: '#E6EAF2' }, d: { accent: '#6A95FF', ink: '#06122F', soft: '#142040', line: '#2A417C', bg: '#0A0E16', s2: '#161C28', s3: '#1E2533' } },
  { id: 'lens', get name() { return t('look.schemeLens') }, l: { accent: '#0B7A6E', ink: '#FFFFFF', soft: '#DDF0EC', line: '#A9D9CF', bg: '#EEF1F4', s2: '#F4F6F8', s3: '#E7EBEF' }, d: { accent: '#3CC2AF', ink: '#04201B', soft: '#12302B', line: '#1E5149', bg: '#0B1015', s2: '#18212A', s3: '#202B35' } },
  { id: 'cobalt', get name() { return t('look.schemeCobalt') }, l: { accent: '#2557D6', ink: '#FFFFFF', soft: '#E2EAFB', line: '#B4C8F3', bg: '#EDF0F6', s2: '#F4F6FA', s3: '#E6EAF2' }, d: { accent: '#86A6FF', ink: '#0A1430', soft: '#17213D', line: '#2A3A66', bg: '#0B0E16', s2: '#171C27', s3: '#1F2532' } },
  { id: 'pine', get name() { return t('look.schemePine') }, l: { accent: '#2D6A3E', ink: '#FFFFFF', soft: '#E0EFE4', line: '#AFD3B8', bg: '#EEF2EF', s2: '#F5F7F5', s3: '#E6ECE7' }, d: { accent: '#72C98D', ink: '#06200E', soft: '#14301D', line: '#24503A', bg: '#0A100C', s2: '#161E18', s3: '#1E2820' } },
  { id: 'amber', get name() { return t('look.schemeAmber') }, l: { accent: '#A9580A', ink: '#FFFFFF', soft: '#F8EAD7', line: '#E6C69A', bg: '#F3F1ED', s2: '#F8F6F2', s3: '#ECE8E1' }, d: { accent: '#F0A44B', ink: '#2A1703', soft: '#33240F', line: '#5A3E17', bg: '#110E0A', s2: '#1E1A14', s3: '#28221A' } },
  { id: 'wine', get name() { return t('look.schemeWine') }, l: { accent: '#9A2B47', ink: '#FFFFFF', soft: '#F6E2E8', line: '#E3B3C1', bg: '#F3EFF1', s2: '#F8F5F6', s3: '#ECE6E9' }, d: { accent: '#F08EA6', ink: '#2E0812', soft: '#361A22', line: '#5C2A39', bg: '#120C0E', s2: '#1F1619', s3: '#291E22' } },
  { id: 'graphite', get name() { return t('look.graphite') }, l: { accent: '#27303B', ink: '#FFFFFF', soft: '#E4E7EB', line: '#C2C8D0', bg: '#EFF0F2', s2: '#F6F6F7', s3: '#E8E9EC' }, d: { accent: '#D3DAE2', ink: '#11161C', soft: '#262D36', line: '#3A434E', bg: '#0C0E11', s2: '#191C20', s3: '#22262B' } },
] as const

export type Theme = 'system' | 'light' | 'dark'

export function applyAppearance(schemeId: string, theme: Theme) {
  const s = SCHEMES.find(x => x.id === schemeId) ?? SCHEMES[0]
  const root = document.documentElement
  for (const [k, v] of Object.entries(s.l)) root.style.setProperty(`--sl-${k === 'ink' ? 'accent-ink' : k}`, v)
  for (const [k, v] of Object.entries(s.d)) root.style.setProperty(`--sd-${k === 'ink' ? 'accent-ink' : k}`, v)
  if (theme === 'system') root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', theme)
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', s.l.accent)
  try { localStorage.setItem('zm-look', JSON.stringify({ scheme: s.id, theme })) } catch {}
}

export function savedAppearance(): { scheme: string, theme: Theme } {
  try {
    const v = JSON.parse(localStorage.getItem('zm-look') || '{}')
    return { scheme: v.scheme || 'tmail', theme: v.theme || 'system' }
  } catch { return { scheme: 'tmail', theme: 'system' } }
}
