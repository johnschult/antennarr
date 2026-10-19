import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from '@/App'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

describe('App routes', () => {
  it.each([
    ['/', 'tuners live'],
    ['/viewers', 'Viewers'],
    ['/tuner?list', 'Tuners'],
    ['/tuner/antenna', 'Steve FLEX 4K'],
    ['/tuner/antenna/firmware', 'Steve FLEX 4K'],
    ['/settings', 'Settings'],
  ])('renders %s inside the shell', (route, heading) => {
    renderWithApp(<App />, makeState(), { route })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(heading)
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument()
  })

  it('opens the only tuner’s page from /tuner, and the list when there are several', () => {
    const { unmount } = renderWithApp(<App />, makeState(), { route: '/tuner' })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Steve FLEX 4K')
    unmount()
    renderWithApp(<App />, makeTwoDeviceState(), { route: '/tuner' })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
  })

  it('sends the old Connect page to Viewers, where adding a viewer now walks through it', () => {
    renderWithApp(<App />, makeState(), { route: '/setup' })
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Viewers')
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

  it('shows whether the live feed is connected', () => {
    const { unmount } = renderWithApp(<App />, makeState(), { connected: true })
    expect(within(screen.getByRole('banner')).getByTestId('connection')).toHaveTextContent('Live')
    unmount()
    renderWithApp(<App />, makeState(), { connected: false })
    expect(within(screen.getByRole('banner')).getByTestId('connection')).toHaveTextContent('Reconnecting…')
  })

  it('says nothing about the tuners in the top bar, however many there are', () => {
    // Each tuner has its header on the Live page and a row on Tuners, and an offline one is a banner: a bar
    // that tried to list them would run out of room.
    const many = makeTwoDeviceState()
    for (let i = 0; i < 4; i++) many.devices.push({ ...many.devices[0], name: `extra${i}`, label: `Extra tuner number ${i}` })
    const offline = makeTwoDeviceState()
    offline.devices[1].tuner.online = false
    const named = makeState()
    named.devices[0].label = 'Living room antenna'
    for (const [what, state] of [['none', makeState({ devices: [] })], ['one', makeState()], ['a named one', named], ['two', makeTwoDeviceState()], ['one offline', offline], ['six', many]] as const) {
      const { unmount } = renderWithApp(<App />, state, { route: '/viewers' })
      const header = within(screen.getByRole('banner'))
      expect(header.getByTestId('connection'), what).toBeInTheDocument()
      for (const text of [/Steve FLEX 4K/, /Cable HDHR/, /Living room antenna/, /Extra tuner/, /192\.168\.1\./, /\d tuners/, /No tuners yet/]) {
        expect(header.queryByText(text), `${what}: ${text}`).not.toBeInTheDocument()
      }
      expect(header.queryByTitle(/online|offline/), what).not.toBeInTheDocument()
      unmount()
    }
  })
})
