// Backgrounds of the webmail, like Gmail themes: ready-made ones are drawn here (gradients and SVG,
// nothing downloaded), "custom" is the user's own picture kept on the server.
// Panels with mail stay solid; the side columns become frosted glass over the picture.

import { t } from '~/composables/i18n'

const svg = (s: string) => `url("data:image/svg+xml,${encodeURIComponent(s)}")`

function mountains(sky: [string, string], layers: string[], sun: string) {
  const ridges = [
    'M0 560 L140 470 L260 520 L420 400 L560 480 L700 380 L860 470 L1000 360 L1160 450 L1300 390 L1460 470 L1600 420 L1600 900 L0 900Z',
    'M0 640 L180 560 L320 610 L480 520 L640 600 L820 500 L980 590 L1140 520 L1320 600 L1480 540 L1600 580 L1600 900 L0 900Z',
    'M0 720 L200 650 L380 700 L560 630 L760 700 L940 640 L1120 710 L1300 650 L1480 700 L1600 670 L1600 900 L0 900Z',
    'M0 800 L240 750 L480 790 L720 740 L960 790 L1200 750 L1440 790 L1600 770 L1600 900 L0 900Z',
  ]
  return svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice">
<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${sky[0]}"/><stop offset="1" stop-color="${sky[1]}"/></linearGradient></defs>
<rect width="1600" height="900" fill="url(#s)"/><circle cx="1180" cy="300" r="90" fill="${sun}" opacity=".85"/>
${ridges.map((d, i) => `<path d="${d}" fill="${layers[i]}"/>`).join('')}</svg>`)
}

function waves(bg: string, colors: string[]) {
  const paths = colors.map((c, i) => {
    const y = 420 + i * 110, a = 60 + i * 12
    return `<path d="M0 ${y} C 260 ${y - a}, 520 ${y + a}, 800 ${y} S 1340 ${y - a}, 1600 ${y} L1600 900 L0 900Z" fill="${c}"/>`
  }).join('')
  return svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice"><rect width="1600" height="900" fill="${bg}"/>${paths}</svg>`)
}

// Contour lines of an imaginary map: rings around a few hills, slightly wobbly.
function topo(bg: string, line: string) {
  const hills = [[300, 260], [1150, 220], [800, 640], [1450, 700], [150, 760]]
  let p = ''
  for (const [cx, cy] of hills) {
    for (let r = 30; r < 420; r += 34) {
      const pts: string[] = []
      for (let k = 0; k <= 48; k++) {
        const ang = k / 48 * Math.PI * 2
        const rr = r * (1 + 0.12 * Math.sin(3 * ang + cx) + 0.07 * Math.cos(5 * ang + cy))
        pts.push(`${(cx + rr * Math.cos(ang)).toFixed(1)},${(cy + rr * 0.72 * Math.sin(ang)).toFixed(1)}`)
      }
      p += `<polyline points="${pts.join(' ')}"/>`
    }
  }
  return svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice"><rect width="1600" height="900" fill="${bg}"/><g fill="none" stroke="${line}" stroke-width="1.4">${p}</g></svg>`)
}

function stars(bg1: string, bg2: string) {
  let dots = ''
  let seed = 7
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647
  for (let i = 0; i < 260; i++) dots += `<circle cx="${(rnd() * 1600).toFixed(0)}" cy="${(rnd() * 900).toFixed(0)}" r="${(rnd() * 1.6 + 0.3).toFixed(1)}" opacity="${(rnd() * 0.7 + 0.2).toFixed(2)}"/>`
  return svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice">
<defs><radialGradient id="g" cx=".7" cy=".2" r="1"><stop offset="0" stop-color="${bg2}"/><stop offset="1" stop-color="${bg1}"/></radialGradient></defs>
<rect width="1600" height="900" fill="url(#g)"/><g fill="#fff">${dots}</g></svg>`)
}

// The base colour is a gradient layer too: a bare colour is only valid as the very last layer of a
// background, and the page puts its own layers after this one.
const mesh = (base: string, spots: [number, number, string][]) =>
  spots.map(([x, y, c]) => `radial-gradient(at ${x}% ${y}%, ${c} 0, transparent 55%)`).join(', ') + `, linear-gradient(${base}, ${base})`

export type Background = { id: string, name: string, css: string, dark?: boolean }

// Names are getters so they follow the interface language.
export const BACKGROUNDS: Background[] = [
  { id: 'aurora', get name() { return t('look.bgAurora') }, css: mesh('#E9EEFF', [[10, 15, '#9DB8FF'], [85, 10, '#D7B6FF'], [70, 85, '#9EE6D8'], [15, 90, '#FFD6E8']]) },
  { id: 'sunrise', get name() { return t('look.bgSunrise') }, css: mesh('#FFF1E6', [[15, 20, '#FFC2A8'], [80, 15, '#FFE29A'], [80, 80, '#FFB3C7'], [20, 85, '#FFD9B0']]) },
  { id: 'lagoon', get name() { return t('look.bgLagoon') }, css: mesh('#E3F6F5', [[20, 10, '#7FD3E6'], [90, 30, '#9BE7C4'], [60, 95, '#6FB8E8'], [5, 75, '#C2F0E4']]) },
  { id: 'lavender', get name() { return t('look.bgLavender') }, css: mesh('#F2EEFB', [[15, 15, '#CDBDF5'], [85, 25, '#F3C5E3'], [50, 90, '#B9C7F7']]) },
  { id: 'mountains', get name() { return t('look.bgMountains') }, css: mountains(['#9CC3EC', '#E8F1FA'], ['#8DA9C9', '#6E8DB3', '#4D6C94', '#2F4C70'], '#FFF4D6') },
  { id: 'dusk', get name() { return t('look.bgDusk') }, css: mountains(['#F6A77A', '#FBD9B8'], ['#C77A79', '#9B5B73', '#6E4467', '#432F55'], '#FFE7A8') },
  { id: 'ocean', get name() { return t('look.bgOcean') }, css: waves('#CFE8F7', ['#9BCDEB', '#6FB0DD', '#3F8CC6', '#1F679F']) },
  { id: 'dunes', get name() { return t('look.bgDunes') }, css: waves('#F7E7CF', ['#EFD2A7', '#E3B983', '#D29E62', '#B98048']) },
  { id: 'topo', get name() { return t('look.bgTopo') }, css: topo('#EEF2F5', '#C6D2DD') },
  { id: 'forest', get name() { return t('look.bgForest') }, css: mesh('#E6F2E8', [[10, 20, '#A9D8B2'], [90, 10, '#CFE8B5'], [70, 90, '#7DBE95'], [10, 90, '#BFE3C6']]) },
  { id: 'night', get name() { return t('look.bgNight') }, css: stars('#0B1230', '#2B3A78'), dark: true },
  { id: 'graphite', get name() { return t('look.graphite') }, css: mesh('#1C2129', [[20, 20, '#2D3644'], [80, 80, '#323B4A'], [80, 10, '#262E3A']]), dark: true },
]

const LS = 'zm-bg'

// applyBackground puts the picture under the app; "" removes it.
export function applyBackground(id: string, version = 0) {
  const root = document.documentElement
  let css = ''
  let dark = false
  if (id === 'custom') css = `url("/api/me/background/image?v=${version}")`
  else {
    const b = BACKGROUNDS.find(x => x.id === id)
    if (b) { css = b.css; dark = !!b.dark }
  }
  if (!css) {
    root.removeAttribute('data-bg')
    root.style.removeProperty('--bg-img')
  } else {
    root.setAttribute('data-bg', dark ? 'dark' : 'light')
    root.style.setProperty('--bg-img', css)
  }
  try { localStorage.setItem(LS, JSON.stringify({ id: css ? id : '', version })) } catch {}
}

// The remembered background is applied at start-up, before /api/me answers (no flash).
export function savedBackground(): { id: string, version: number } {
  try { const v = JSON.parse(localStorage.getItem(LS) || '{}'); return { id: v.id || '', version: v.version || 0 } } catch { return { id: '', version: 0 } }
}
