import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from '@/App'
import { makeState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

describe('App routes', () => {
  it.each([
    ['/', 'tuners live'],
    ['/viewers', 'Viewers'],
    ['/setup', 'Setup guide'],
    ['/tuner', 'Tuner'],
    ['/settings', 'Settings'],
  ])('renders %s inside the shell', (route, heading) => {
    renderWithApp(<App />, makeState(), { route })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(heading)
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument()
  })

  it('sends unknown paths to the live page', () => {
    renderWithApp(<App />, makeState(), { route: '/nope/nothing' })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('tuners live')
  })

  it('marks the current page in the navigation', () => {
    renderWithApp(<App />, makeState(), { route: '/viewers' })
    expect(screen.getByRole('link', { name: 'Viewers' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Live' })).not.toHaveAttribute('aria-current')
  })

  it('shows the tuner and whether the live feed is connected', () => {
    const { unmount } = renderWithApp(<App />, makeState(), { connected: true })
    expect(screen.getByTestId('connection')).toHaveTextContent('Live')
    expect(screen.getByText(/Steve FLEX 4K/)).toBeInTheDocument()
    unmount()
    renderWithApp(<App />, makeState(), { connected: false })
    expect(screen.getByTestId('connection')).toHaveTextContent('Reconnecting…')
  })
})
