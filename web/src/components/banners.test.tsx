import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { Banners } from './banners'

const show = (s = makeState()) =>
  render(
    <MemoryRouter>
      <Banners state={s} />
    </MemoryRouter>,
  )

describe('Banners', () => {
  it('shows nothing when all is well', () => {
    show()
    expect(screen.queryAllByRole('status')).toHaveLength(0)
  })

  it('says nobody can use the tuner while there are no viewers', () => {
    const s = makeState({ viewers: [] })
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Nobody can use your tuner yet')
    expect(screen.getByRole('link', { name: 'Add a viewer' })).toHaveAttribute('href', '/viewers?add')
  })

  it('says the tuner is offline and what Yagi will do about it', () => {
    const s = makeState()
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent("The tuner isn't answering at 192.168.1.132")
    expect(screen.getByRole('status')).toHaveTextContent('Yagi will keep looking')
  })

  it('says what Yagi did to find the tuner once it has looked and found nothing', () => {
    const s = makeState()
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).not.toHaveTextContent('searched the network')
    s.devices[0].tuner.search = { at: s.time, found: false }
    show(s)
    expect(screen.getAllByRole('status')[1]).toHaveTextContent('searched the network for it and found nothing. Is it powered on and on the same network?')
  })

  it('links to the docs site, not the old docs folder', () => {
    const s = makeState()
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getByRole('link', { name: 'Troubleshooting' })).toHaveAttribute('href', 'https://yagi.johnschult.com/troubleshooting/#the-tuner-shows-as-offline')
  })

  it('says when the tuner turned a stream away, in the words of a media server, for a while', () => {
    const s = makeState()
    const at = new Date(new Date(s.time).getTime() - 3 * 60 * 1000).toISOString()
    s.devices[0].tuner.problems = [{ kind: 'refused', time: at, last: at, text: 'The tuner turned a stream away: 503 Service Unavailable (it said: 805 - Tuner in use)' }]
    show(s)
    const note = screen.getByRole('status')
    expect(note).toHaveTextContent('The tuner turned a stream away: 503 Service Unavailable (it said: 805 - Tuner in use).')
    expect(note).toHaveTextContent('"channel not available"')
    expect(note).toHaveTextContent('3m ago')
  })

  it('stops showing a refused stream after fifteen minutes', () => {
    const s = makeState()
    const old = new Date(new Date(s.time).getTime() - 16 * 60 * 1000).toISOString()
    s.devices[0].tuner.problems = [
      { kind: 'refused', time: old, last: old, text: 'The tuner turned a stream away: 503' },
      { kind: 'denied', time: old, last: old, text: 'John was refused a tuner: all shared tuners are in use' },
    ]
    show(s)
    expect(screen.queryAllByRole('status')).toHaveLength(0)
  })

  it("says when Yagi turned a viewer's stream away", () => {
    const s = makeState()
    s.devices[0].tuner.problems = [{ kind: 'denied', time: s.time, last: s.time, text: 'John was refused a tuner: the remaining tuners are reserved for the owner' }]
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('John was refused a tuner: the remaining tuners are reserved for the owner.')
    expect(screen.getByRole('link', { name: 'Troubleshooting' })).toHaveAttribute('href', 'https://yagi.johnschult.com/troubleshooting/#a-503-when-watching')
  })

  it('keeps saying the channel list is empty until it is not, however long ago it was', () => {
    const s = makeState()
    const old = new Date(new Date(s.time).getTime() - 3 * 60 * 60 * 1000).toISOString()
    s.devices[0].tuner.problems = [{ kind: 'no-channels', time: old, last: old, text: "The tuner answered a media server's request for its channels with an empty list" }]
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('empty list')
    expect(screen.getByRole('status')).toHaveTextContent('guide in a media server looks empty')
  })

  it("explains why changes can't be saved when there is no settings file", () => {
    show(makeState({ editable: false }))
    expect(screen.getByRole('status')).toHaveTextContent('-config')
  })

  it('can show several at once', () => {
    const s = makeState({ editable: false })
    s.devices[0].tuner.online = false
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getAllByRole('status')).toHaveLength(3)
  })

  it('names the offline tuners when there are several devices', () => {
    const s = makeTwoDeviceState()
    s.devices[1].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('A tuner is offline: Cable HDHR.')
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getAllByRole('status')[1]).toHaveTextContent('2 tuners are offline: Steve FLEX 4K, Cable HDHR.')
  })

  it('talks about tuners in the plural with no viewers and several devices', () => {
    const s = makeTwoDeviceState()
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Nobody can use your tuners yet.')
  })

  it('names offline tuners by the name the owner gave them', () => {
    const s = makeTwoDeviceState()
    s.devices[1].label = 'Cable box'
    s.devices[1].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('A tuner is offline: Cable box.')
  })
})

describe('the offline banner', () => {
  it('links each offline tuner’s name to its page', () => {
    const state = makeTwoDeviceState()
    state.devices[0].tuner.online = false
    state.devices[1].tuner.online = false
    render(
      <MemoryRouter>
        <Banners state={state} />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: 'Steve FLEX 4K' })).toHaveAttribute('href', '/tuners/antenna')
    expect(screen.getByRole('link', { name: 'Cable HDHR' })).toHaveAttribute('href', '/tuners/cable')
  })

  it('links the one tuner’s offline notice to its page', () => {
    const state = makeState()
    state.devices[0].tuner.online = false
    render(
      <MemoryRouter>
        <Banners state={state} />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: /The tuner isn't answering at/ })).toHaveAttribute('href', '/tuners/antenna')
  })
})

describe('the notice about the address', () => {
  const claimed = () => {
    const s = makeState()
    s.settings.admin = ['192.168.1.20']
    s.settings.plex.enabled = false
    return s
  }

  it('warns that an address can change, and points to a router reservation, Plex sign-in and the docs', () => {
    show(claimed())
    expect(screen.getByRole('status')).toHaveTextContent("Your way into this page is this computer's address")
    expect(screen.getByRole('status')).toHaveTextContent('Reserve its address in your router')
    expect(screen.getByRole('link', { name: 'turn on Sign in with Plex' })).toHaveAttribute('href', '/settings')
    expect(screen.getByRole('link', { name: 'Troubleshooting' })).toHaveAttribute('href', 'https://yagi.johnschult.com/troubleshooting/#this-address-cant-open-the-admin-page')
  })

  it('is not shown once Plex sign-in is on, or when nobody was let in by address', () => {
    const on = claimed()
    on.settings.plex.enabled = true
    show(on)
    expect(screen.queryAllByRole('status')).toHaveLength(0)
  })

  it('can be dismissed, and this browser stays quiet about it', async () => {
    const { default: userEvent } = await import('@testing-library/user-event')
    localStorage.removeItem('yagi.dismissedNotices')
    const first = show(claimed())
    await userEvent.setup().click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryAllByRole('status')).toHaveLength(0)
    first.unmount()
    show(claimed())
    expect(screen.queryAllByRole('status')).toHaveLength(0)
    localStorage.removeItem('yagi.dismissedNotices')
  })
})

describe('dismissing a problem', () => {
  const trouble = (time: string, last: string) => {
    const s = makeState()
    s.devices[0].tuner.problems = [{ kind: 'denied', time, last, text: 'John was refused a tuner: the remaining tuners are held for your own devices' }]
    return s
  }

  it('hides that episode, even as the media server retries, and shows the next one', async () => {
    const { default: userEvent } = await import('@testing-library/user-event')
    localStorage.removeItem('yagi.dismissedNotices')
    const start = new Date(makeState().time).getTime()
    const at = (secs: number) => new Date(start + secs * 1000).toISOString()

    const first = show(trouble(at(-60), at(-60)))
    await userEvent.setup().click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryAllByRole('status')).toHaveLength(0)
    first.unmount()

    // It happened again a few seconds later: still the same episode, so it stays hidden.
    const retry = show(trouble(at(-60), at(-5)))
    expect(screen.queryAllByRole('status')).toHaveLength(0)
    retry.unmount()

    // Another episode, later, with a different start: shown again.
    show(trouble(at(-20), at(-20)))
    expect(screen.getByRole('status')).toHaveTextContent('John was refused a tuner')
    localStorage.removeItem('yagi.dismissedNotices')
  })

  it('goes by when it last happened, not when it started, for how long it is shown', () => {
    const s = makeState()
    const now = new Date(s.time).getTime()
    // Started an hour ago but happened again two minutes ago: still going on.
    s.devices[0].tuner.problems = [{ kind: 'refused', time: new Date(now - 3600_000).toISOString(), last: new Date(now - 120_000).toISOString(), text: 'The tuner turned a stream away: 503' }]
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('2m ago')
  })
})

