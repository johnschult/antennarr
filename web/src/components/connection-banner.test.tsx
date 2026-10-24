import { act, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { makeState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { ConnectionBanner } from './banners'

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('the banner for a page that lost Yagi', () => {
  it('says nothing while connected', () => {
    renderWithApp(<ConnectionBanner />, makeState(), { connected: true })
    act(() => void vi.advanceTimersByTime(10_000))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('waits a few seconds, so a quick reconnect is not news', () => {
    renderWithApp(<ConnectionBanner />, makeState(), { connected: false })
    act(() => void vi.advanceTimersByTime(2_000))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('then says what it means and what the page is doing about it', () => {
    renderWithApp(<ConnectionBanner />, makeState(), { connected: false })
    act(() => void vi.advanceTimersByTime(3_500))
    const b = screen.getByRole('status')
    expect(b).toHaveTextContent("This page can't reach Yagi.")
    expect(b).toHaveTextContent('changes can\'t be saved')
    expect(b).toHaveTextContent('keeps trying by itself')
    expect(screen.getByRole('link', { name: 'Troubleshooting' })).toHaveAttribute('href', expect.stringContaining('reconnecting'))
  })
})
