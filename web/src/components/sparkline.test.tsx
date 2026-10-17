import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Sparkline } from './sparkline'

describe('Sparkline', () => {
  it('draws a line through the samples', () => {
    const { container } = render(<Sparkline data={[0, 10, 5]} label="upload" />)
    const paths = container.querySelectorAll('path')
    expect(paths).toHaveLength(2) // the fill and the line
    expect(screen.getByRole('img', { name: 'upload' })).toBeInTheDocument()
    expect(paths[1].getAttribute('d')).toMatch(/^M0\.0 \d+\.\d L150\.0 .* L300\.0 /)
  })

  it('scales to the largest sample unless told otherwise', () => {
    const y = (max?: number) => {
      const { container, unmount } = render(<Sparkline data={[0, 10]} height={54} max={max} />)
      const d = container.querySelectorAll('path')[1].getAttribute('d')!
      unmount()
      return Number(d.split(' ').pop())
    }
    expect(y()).toBeCloseTo(2) // the peak touches the top
    expect(y(20)).toBeGreaterThan(y()) // a taller scale leaves headroom
  })

  it('renders an empty placeholder until there are two samples', () => {
    const { container, rerender } = render(<Sparkline data={[]} />)
    expect(screen.getByRole('img')).toHaveAccessibleName(/no data yet/)
    expect(container.querySelector('path')).toBeNull()
    rerender(<Sparkline data={[3]} />)
    expect(container.querySelector('path')).toBeNull()
  })
})
