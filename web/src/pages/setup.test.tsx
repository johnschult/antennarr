import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { SetupPage } from './setup'

describe('SetupPage (placeholder)', () => {
  it("shows the address a viewer would enter", () => {
    renderWithApp(<SetupPage />, makeState())
    expect(screen.getByText(`${window.location.hostname}:5004`)).toBeInTheDocument()
  })

  it('reminds you to add a viewer first when there are none', () => {
    const { unmount } = renderWithApp(<SetupPage />, makeState({ viewers: [] }))
    expect(screen.getByText(/Add a viewer first/)).toBeInTheDocument()
    unmount()
    renderWithApp(<SetupPage />, makeState())
    expect(screen.queryByText(/Add a viewer first/)).not.toBeInTheDocument()
  })

  it('gives each device its own address', () => {
    renderWithApp(<SetupPage />, makeTwoDeviceState())
    expect(screen.getByText(`${window.location.hostname}:5004`)).toBeInTheDocument()
    expect(screen.getByText(`${window.location.hostname}:5005`)).toBeInTheDocument()
    expect(screen.getByText('antenna')).toBeInTheDocument()
    expect(screen.getByText('cable')).toBeInTheDocument()
    expect(screen.getByText(/adds each tuner/)).toBeInTheDocument()
  })
})
