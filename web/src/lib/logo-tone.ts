/** What a channel's logo is mostly made of, which decides what to put it on. */
export type LogoTone = 'light' | 'dark' | 'mixed'

/**
 * Looks at a logo's pixels (RGBA, as a canvas gives them) and says whether what is drawn is mostly light
 * (white lettering, which vanishes on a light background), mostly dark (which vanishes on a dark one), or
 * neither. Transparent pixels are the background, not the logo, and are not counted.
 */
export function logoTone(data: ArrayLike<number>): LogoTone {
  let seen = 0
  let light = 0
  let dark = 0
  for (let i = 0; i + 3 < data.length; i += 4) {
    if (data[i + 3] < 128) continue
    const l = (0.2126 * data[i] + 0.7152 * data[i + 1] + 0.0722 * data[i + 2]) / 255
    seen++
    if (l > 0.8) light++
    else if (l < 0.2) dark++
  }
  if (seen === 0) return 'mixed'
  return light / seen > 0.6 ? 'light' : dark / seen > 0.6 ? 'dark' : 'mixed'
}

const known = new Map<string, LogoTone>()

/** What an address's tone was found to be, if it has been looked at already. */
export const knownTone = (src: string): LogoTone | undefined => known.get(src)

/**
 * The tone of a loaded logo. The logo is served by Antennarr, so the canvas can read it; if it can't be
 * read (no canvas, or the browser refuses), it is "mixed", which is the neutral choice.
 */
export function toneOfImage(src: string, img: HTMLImageElement): LogoTone {
  const cached = known.get(src)
  if (cached) return cached
  let tone: LogoTone = 'mixed'
  try {
    const canvas = document.createElement('canvas')
    canvas.width = 64
    canvas.height = 48
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (ctx) {
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
      tone = logoTone(ctx.getImageData(0, 0, canvas.width, canvas.height).data)
    }
  } catch {
    tone = 'mixed'
  }
  known.set(src, tone)
  return tone
}
