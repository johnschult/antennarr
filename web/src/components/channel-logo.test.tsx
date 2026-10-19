import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ChannelLogo } from './channel-logo'

afterEach(() => vi.restoreAllMocks())

const withPixels = (rgba: [number, number, number, number]) =>
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation((() => ({
    drawImage: () => {},
    getImageData: () => ({ data: Uint8ClampedArray.from(Array.from({ length: 20 }, () => rgba).flat()) }),
  })) as unknown as typeof HTMLCanvasElement.prototype.getContext)

const logo = () => screen.getByRole('img', { name: 'WSB logo' })

describe('ChannelLogo', () => {
  it('shows the logo with no outline until it has looked at it, and for a mix of colors', () => {
    withPixels([200, 40, 40, 255])
    render(<ChannelLogo src="/l/mixed.png" name="WSB" />)
    expect(logo()).toHaveAttribute('data-tone', 'mixed')
    fireEvent.load(logo())
    expect(logo()).toHaveAttribute('data-tone', 'mixed')
    expect(logo().className).not.toMatch(/drop-shadow/)
  })

  it('outlines white artwork in dark in the light theme, and in nothing in the dark theme', () => {
    withPixels([255, 255, 255, 255])
    render(<ChannelLogo src="/l/white.png" name="WSB" />)
    fireEvent.load(logo())
    expect(logo()).toHaveAttribute('data-tone', 'light')
    expect(logo().className).toContain('[filter:drop-shadow(0_0_1px_rgb(0_0_0/0.8))')
    expect(logo().className).toContain('dark:[filter:none]')
  })

  it('outlines dark artwork in light, in the dark theme only', () => {
    withPixels([0, 0, 0, 255])
    render(<ChannelLogo src="/l/black.png" name="WSB" />)
    fireEvent.load(logo())
    expect(logo()).toHaveAttribute('data-tone', 'dark')
    expect(logo().className).toContain('dark:[filter:drop-shadow(0_0_1px_rgb(255_255_255/0.85))')
    expect(logo().className).not.toMatch(/(^|\s)\[filter:drop-shadow/)
  })

  it('is just the logo, with no box around it', () => {
    withPixels([255, 255, 255, 255])
    const { container } = render(<ChannelLogo src="/l/nobox.png" name="WSB" />)
    expect(container.firstElementChild?.tagName).toBe('IMG')
  })

  it('has nothing at all when the logo will not load', () => {
    const { container } = render(<ChannelLogo src="/l/missing.png" name="WSB" />)
    fireEvent.error(logo())
    expect(container).toBeEmptyDOMElement()
  })

  it('has a size for a row in a table, and for a tile', () => {
    const { rerender } = render(<ChannelLogo src="/l/size.png" name="WSB" />)
    expect(logo().className).toContain('h-8')
    rerender(<ChannelLogo src="/l/size.png" name="WSB" size="sm" />)
    expect(logo().className).toContain('h-6')
  })
})
