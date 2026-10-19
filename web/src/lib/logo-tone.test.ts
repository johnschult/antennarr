import { afterEach, describe, expect, it, vi } from 'vitest'
import { knownTone, logoTone, toneOfImage } from './logo-tone'

/** RGBA pixels from [r, g, b, a] tuples. */
const px = (...p: [number, number, number, number][]) => Uint8ClampedArray.from(p.flat())
const many = (n: number, p: [number, number, number, number]) => Array.from({ length: n }, () => p)

describe('logoTone', () => {
  it('calls mostly white artwork light', () => {
    expect(logoTone(px(...many(9, [255, 255, 255, 255]), [200, 30, 30, 255]))).toBe('light')
  })

  it('calls mostly black artwork dark', () => {
    expect(logoTone(px(...many(9, [5, 5, 10, 255]), [250, 250, 250, 255]))).toBe('dark')
  })

  it('calls a mix of colors mixed', () => {
    expect(logoTone(px(...many(3, [255, 255, 255, 255]), ...many(3, [0, 0, 0, 255]), ...many(4, [200, 40, 40, 255])))).toBe('mixed')
  })

  it('calls a logo with a good share of white lettering light, as the NBC logo (about 47% near-white) is', () => {
    expect(logoTone(px(...many(47, [255, 255, 255, 255]), ...many(53, [200, 60, 40, 255])))).toBe('light')
    expect(logoTone(px(...many(39, [255, 255, 255, 255]), ...many(61, [200, 60, 40, 255])))).toBe('mixed')
  })

  it('does not count transparent pixels, which are the background and not the logo', () => {
    // a lot of transparent black around a little white lettering is still a white logo
    expect(logoTone(px(...many(50, [0, 0, 0, 0]), ...many(5, [255, 255, 255, 255])))).toBe('light')
    expect(logoTone(px(...many(50, [255, 255, 255, 0]), ...many(5, [0, 0, 0, 255])))).toBe('dark')
  })

  it('does not count mostly transparent edge pixels', () => {
    expect(logoTone(px(...many(50, [255, 255, 255, 100]), ...many(5, [0, 0, 0, 255])))).toBe('dark')
  })

  it('is mixed for an empty or fully transparent image', () => {
    expect(logoTone(px())).toBe('mixed')
    expect(logoTone(px(...many(10, [255, 255, 255, 0])))).toBe('mixed')
  })

  it('weighs colors the way eyes do: pure blue is dark, pure green is not', () => {
    expect(logoTone(px(...many(10, [0, 0, 255, 255])))).toBe('dark')
    expect(logoTone(px(...many(10, [0, 255, 0, 255])))).toBe('mixed')
  })
})

describe('toneOfImage', () => {
  afterEach(() => vi.restoreAllMocks())

  const canvasOf = (data: Uint8ClampedArray | null) =>
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      (() => (data ? { drawImage: () => {}, getImageData: () => ({ data }) } : null)) as unknown as typeof HTMLCanvasElement.prototype.getContext,
    )

  it('reads the logo from a canvas, and remembers it for that address', () => {
    const spy = canvasOf(px(...many(10, [255, 255, 255, 255])))
    const img = document.createElement('img')
    expect(toneOfImage('/logo-a.png', img)).toBe('light')
    expect(knownTone('/logo-a.png')).toBe('light')
    expect(toneOfImage('/logo-a.png', img)).toBe('light')
    expect(spy).toHaveBeenCalledTimes(1)
  })

  it('is mixed when there is no canvas to read it with', () => {
    canvasOf(null)
    expect(toneOfImage('/logo-b.png', document.createElement('img'))).toBe('mixed')
  })

  it('is mixed when the browser refuses to let the canvas be read', () => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation((() => ({
      drawImage: () => {},
      getImageData: () => {
        throw new DOMException('tainted', 'SecurityError')
      },
    })) as unknown as typeof HTMLCanvasElement.prototype.getContext)
    expect(toneOfImage('/logo-c.png', document.createElement('img'))).toBe('mixed')
  })
})
