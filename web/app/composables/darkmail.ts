// Dark rendering of HTML mail, similar to Outlook/Apple Mail: light backgrounds are darkened and
// dark text is lightened by inverting HSL lightness while keeping hue, so branded colours stay
// recognisable. Images and elements with background images are left untouched.
type RGB = [number, number, number]

function parse(c: string): { rgb: RGB, a: number } | null {
  const m = c.match(/rgba?\(([^)]+)\)/)
  if (!m) return null
  const p = m[1].split(/[\s,/]+/).filter(Boolean).map(Number)
  return { rgb: [p[0], p[1], p[2]], a: p.length > 3 ? p[3] : 1 }
}

function toHsl([r, g, b]: RGB): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2
  if (max === min) return [0, 0, l]
  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  const h = max === r ? (g - b) / d + (g < b ? 6 : 0) : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [h * 60, s, l]
}

function hsl(h: number, s: number, l: number) {
  return `hsl(${h.toFixed(0)} ${(s * 100).toFixed(0)}% ${(l * 100).toFixed(1)}%)`
}

const lum = ([r, g, b]: RGB) => (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255

export function darkenMail(doc: Document) {
  const win = doc.defaultView
  if (!win || !doc.body) return
  const els = doc.body.querySelectorAll<HTMLElement>('*')
  const max = Math.min(els.length, 4000)
  for (let i = 0; i < max; i++) {
    const el = els[i]
    if (el.tagName === 'IMG' || el.tagName === 'svg' || el.closest('svg')) continue
    const cs = win.getComputedStyle(el)
    const bg = parse(cs.backgroundColor)
    if (bg && bg.a > 0.05 && cs.backgroundImage === 'none' && lum(bg.rgb) > 0.45) {
      const [h, s, l] = toHsl(bg.rgb)
      const grey = s < 0.12
      el.style.setProperty('background-color', hsl(grey ? 215 : h, grey ? 0.22 : s * 0.55, 0.1 + (1 - l) * 0.32), 'important')
    }
    const fg = parse(cs.color)
    if (fg && lum(fg.rgb) < 0.55) {
      const [h, s, l] = toHsl(fg.rgb)
      el.style.setProperty('color', hsl(h, Math.min(1, s * 0.85), Math.min(0.93, 0.93 - l * 0.35)), 'important')
    }
    const bc = parse(cs.borderTopColor)
    if (bc && cs.borderTopStyle !== 'none' && lum(bc.rgb) > 0.6) {
      el.style.setProperty('border-color', 'rgba(255,255,255,.14)', 'important')
    }
  }
}
