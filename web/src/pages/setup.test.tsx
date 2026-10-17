import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { makeState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { SetupPage } from './setup'

describe('SetupPage (placeholder)', () => {
  it("shows the address a viewer would enter", () => {
    renderWithApp(<SetupPage />, makeState())
    expect(screen.getByText(window.location.host)).toBeInTheDocument()
  })

  it('reminds you to add a viewer first when there are none', () => {
    const { unmount } = renderWithApp(<SetupPage />, makeState({ viewers: [] }))
    expect(screen.getByText(/Add a viewer first/)).toBeInTheDocument()
    unmount()
    renderWithApp(<SetupPage />, makeState())
    expect(screen.queryByText(/Add a viewer first/)).not.toBeInTheDocument()
  })
})
