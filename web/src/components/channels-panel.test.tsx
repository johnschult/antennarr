import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ChannelsDoc, TunerChannel } from '@/lib/types'
import { makeDevice, makeState } from '@/test/fixtures'
import { renderTunerRoutes, stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

const ch = (number: string, name: string, over: Partial<TunerChannel> = {}): TunerChannel => ({ number, name, state: 'enabled', subscribed: true, ...over })
const channels = [
  ch('11.1', 'WXIA-HD', { hd: true, video: 'MPEG2' }),
  ch('2.1', 'WSB-HD', { hd: true, video: 'MPEG2' }),
  ch('2.2', 'Laff', { state: 'favorite' }),
  ch('2.10', 'Tenth'),
  ch('10.3', 'Old Station', { state: 'disabled' }),
  ch('703', 'Locked', { subscribed: false }),
]
const doc = (over: Partial<ChannelsDoc> = {}): ChannelsDoc => ({ channels, scan: { inProgress: false, possible: true, source: 'Antenna', sources: ['Antenna', 'Cable'] }, ...over })

const path = 'GET /devices/antenna/channels'
function show(d: ChannelsDoc | { status: number; error: string }, extra: Record<string, (b: unknown) => { status?: number; body?: unknown }> = {}, state = makeState()) {
  const { fn, calls } = stubFetch({ [path]: () => ('status' in d ? { status: d.status, body: { error: d.error } } : { body: d }), ...extra })
  vi.stubGlobal('fetch', fn)
  renderTunerRoutes(state, '/tuner/antenna/channels')
  return calls
}
const rows = () => screen.getAllByTestId('channel')
const row = (name: string) => rows().find((r) => within(r).queryByText(name))!

describe('Channels', () => {
  it('lists every channel the tuner has found, in channel-number order, with its state', async () => {
    show(doc())
    await screen.findAllByTestId('channel')
    expect(rows().map((r) => within(r).getAllByText(/./)[0].textContent)).toEqual(['2.1', '2.2', '2.10', '10.3', '11.1', '703'])
    expect(row('WSB-HD')).toHaveAttribute('data-state', 'enabled')
    expect(row('Laff')).toHaveAttribute('data-state', 'favorite')
    expect(row('Old Station')).toHaveAttribute('data-state', 'disabled')
    expect(within(row('WSB-HD')).getByText('HD')).toBeInTheDocument()
    expect(screen.getByTestId('counts')).toHaveTextContent('5 in the lineup · 1 favorite · 1 disabled')
  })

  it('marks which of the three states a channel is in, as pressed buttons', async () => {
    show(doc())
    await screen.findAllByTestId('channel')
    const group = within(row('Laff')).getByRole('group', { name: 'Channel 2.2 Laff' })
    expect(within(group).getByRole('button', { name: 'Favorite' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(group).getByRole('button', { name: 'Enabled' })).toHaveAttribute('aria-pressed', 'false')
    expect(within(group).getByRole('button', { name: 'Disabled' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('disables a channel, showing it at once and sending only that change', async () => {
    const calls = show(doc(), { 'PUT /devices/antenna/channels/2.1': () => ({ body: ch('2.1', 'WSB-HD', { state: 'disabled' }) }) })
    await screen.findAllByTestId('channel')
    await userEvent.setup().click(within(row('WSB-HD')).getByRole('button', { name: 'Disabled' }))
    expect(row('WSB-HD')).toHaveAttribute('data-state', 'disabled')
    expect(screen.getByTestId('counts')).toHaveTextContent('4 in the lineup · 1 favorite · 2 disabled')
    await waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toBeDefined())
    expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(1)
    expect(calls.find((c) => c.method === 'PUT')).toMatchObject({ url: expect.stringContaining('/devices/antenna/channels/2.1'), body: { state: 'disabled' } })
  })

  it('puts it back and says why when the tuner would not change it', async () => {
    show(doc(), { 'PUT /devices/antenna/channels/2.1': () => ({ status: 502, body: { error: "the tuner didn't change it" } }) })
    await screen.findAllByTestId('channel')
    await userEvent.setup().click(within(row('WSB-HD')).getByRole('button', { name: 'Disabled' }))
    expect(await screen.findByText("the tuner didn't change it")).toBeInTheDocument()
    await waitFor(() => expect(row('WSB-HD')).toHaveAttribute('data-state', 'enabled'))
    expect(screen.getByTestId('counts')).toHaveTextContent('5 in the lineup · 1 favorite · 1 disabled')
  })

  it('sends nothing for the state a channel is already in', async () => {
    const calls = show(doc())
    await screen.findAllByTestId('channel')
    await userEvent.setup().click(within(row('WSB-HD')).getByRole('button', { name: 'Enabled' }))
    expect(calls.filter((c) => c.method === 'PUT')).toHaveLength(0)
  })

  it('cannot change a channel the tuner has but cannot play', async () => {
    show(doc())
    await screen.findAllByTestId('channel')
    const r = row('Locked')
    expect(within(r).getByText('not available to this tuner')).toBeInTheDocument()
    for (const b of within(r).getAllByRole('button')) expect(b).toBeDisabled()
  })

  it('filters by state', async () => {
    show(doc())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    const filters = within(screen.getByRole('group', { name: 'Show' }))
    await user.click(filters.getByRole('button', { name: 'Disabled' }))
    expect(rows()).toHaveLength(1)
    expect(rows()[0]).toHaveAttribute('data-state', 'disabled')
    await user.click(filters.getByRole('button', { name: 'Favorites' }))
    expect(rows().map((r) => r.getAttribute('data-state'))).toEqual(['favorite'])
    await user.click(filters.getByRole('button', { name: 'All' }))
    expect(rows()).toHaveLength(6)
    expect(filters.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('searches by number or name, together with the filter', async () => {
    show(doc())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    await user.type(screen.getByRole('textbox', { name: 'Search channels' }), 'wsb')
    expect(rows()).toHaveLength(1)
    await user.clear(screen.getByRole('textbox', { name: 'Search channels' }))
    await user.type(screen.getByRole('textbox', { name: 'Search channels' }), '2.')
    expect(rows().map((r) => within(r).getAllByText(/./)[0].textContent)).toEqual(['2.1', '2.2', '2.10'])
    await user.click(within(screen.getByRole('group', { name: 'Show' })).getByRole('button', { name: 'Favorites' }))
    expect(rows()).toHaveLength(1)
    await user.clear(screen.getByRole('textbox', { name: 'Search channels' }))
    await user.type(screen.getByRole('textbox', { name: 'Search channels' }), 'nothing like this')
    expect(screen.getByText('No channel matches.')).toBeInTheDocument()
  })

  it('says when the tuner has found no channels', async () => {
    show(doc({ channels: [] }))
    expect(await screen.findByText(/has not found any channels/)).toBeInTheDocument()
  })

  it('says why when the tuner could not be read', async () => {
    show({ status: 502, error: 'couldn’t reach the tuner' })
    expect(await screen.findByRole('alert')).toHaveTextContent('couldn’t reach the tuner')
  })

  it('does not ask an offline tuner', () => {
    const state = makeState({ devices: [makeDevice({ tuner: { ...makeDevice().tuner, online: false } })] })
    const calls = show(doc(), {}, state)
    expect(screen.getByText(/The tuner is offline/)).toBeInTheDocument()
    expect(calls.filter((c) => c.url.includes('/channels'))).toHaveLength(0)
  })

  it('shows only the first 300 of a long list, and says so', async () => {
    const many = Array.from({ length: 450 }, (_, i) => ch(String(100 + i), `Channel ${i}`))
    show(doc({ channels: many }))
    await screen.findAllByTestId('channel')
    expect(rows()).toHaveLength(300)
    expect(screen.getByText(/Showing the first 300 of 450/)).toBeInTheDocument()
  })
})

describe('reception', () => {
  const at = (h: number, m = 0) => new Date(2026, 9, 19, h, m).toISOString()
  const session = (channel: string, quality: number, over: Record<string, unknown> = {}) => ({
    device: 'antenna', tuner: 'tuner0', channel, channelName: 'X', kind: 'viewer', viewer: 'John', start: at(8), end: at(9), quality, ...over,
  })

  it('shows the average signal quality of the channels that have been watched, colored when it is not good', async () => {
    show(doc(), {
      'GET /history?days=30': () => ({ body: { days: 30, now: at(20), sessions: [session('2.1', 95), session('2.2', 60), session('10.3', 30), session('2.1', 85, { start: at(10), end: at(11) })] } }),
    })
    await screen.findAllByTestId('channel')
    await waitFor(() => expect(within(row('WSB-HD')).getByTestId('reception')).toHaveTextContent('90%'))
    expect(within(row('WSB-HD')).getByText('90%')).not.toHaveClass('text-amber-700', 'text-red-700')
    expect(within(row('Laff')).getByText('60%')).toHaveClass('text-amber-700')
    expect(within(row('Old Station')).getByText('30%')).toHaveClass('text-red-700')
    expect(within(row('Tenth')).getByTestId('reception')).toBeEmptyDOMElement() // never watched
  })

  it('leaves it out, and the list as it was, when the history cannot be read', async () => {
    show(doc(), { 'GET /history?days=30': () => ({ status: 500, body: { error: 'boom' } }) })
    await screen.findAllByTestId('channel')
    await new Promise((r) => setTimeout(r, 30))
    expect(rows()).toHaveLength(6)
    expect(screen.queryByText(/\d+%$/)).not.toBeInTheDocument()
  })

  it('does not ask for the history, or show the column, when it is switched off', async () => {
    const state = makeState()
    state.settings.history = false
    const calls = show(doc(), {}, state)
    await screen.findAllByTestId('channel')
    expect(screen.queryByTestId('reception')).not.toBeInTheDocument()
    expect(calls.filter((c) => c.url.includes('/history'))).toHaveLength(0)
  })

  it('only counts the sessions on this tuner', async () => {
    show(doc(), { 'GET /history?days=30': () => ({ body: { days: 30, now: at(20), sessions: [session('2.1', 20, { device: 'other' })] } }) })
    await screen.findAllByTestId('channel')
    await new Promise((r) => setTimeout(r, 30))
    expect(within(row('WSB-HD')).getByTestId('reception')).toBeEmptyDOMElement()
  })
})

describe('scanning', () => {
  const idle = () => {
    const s = makeState()
    s.devices[0].summary.inUse = 0
    return s
  }
  const busy = () => {
    const s = makeState()
    s.devices[0].summary.inUse = 2
    return s
  }

  it('asks first, saying a scan takes the tuners, and sends nothing until you agree', async () => {
    const calls = show(doc(), { 'POST /devices/antenna/channel-scan': () => ({ status: 202, body: { scanning: true, source: 'Antenna' } }) }, idle())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Scan for channels' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/uses all of its tuners/)).toBeInTheDocument()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(0)

    await user.click(within(dialog).getByRole('button', { name: 'Scan' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ source: 'Antenna', interrupt: false }))
  })

  it('offers the sources the tuner has, and sends the one chosen', async () => {
    const calls = show(doc(), { 'POST /devices/antenna/channel-scan': () => ({ status: 202, body: {} }) }, idle())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Scan for channels' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.selectOptions(within(dialog).getByLabelText('Scan'), 'Cable')
    await user.click(within(dialog).getByRole('button', { name: 'Scan' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ source: 'Cable' }))
  })

  it('says what a scan will cut off, and sends the interrupt only because you were told', async () => {
    const calls = show(doc(), { 'POST /devices/antenna/channel-scan': () => ({ status: 202, body: {} }) }, busy())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Scan for channels' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByRole('alert')).toHaveTextContent('2 of its tuners are in use right now.')
    await user.click(within(dialog).getByRole('button', { name: 'Interrupt and scan' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ source: 'Antenna', interrupt: true }))
  })

  it('stays open with the reason when the server refuses', async () => {
    show(doc(), { 'POST /devices/antenna/channel-scan': () => ({ status: 409, body: { error: 'a scan is already running' } }) }, idle())
    await screen.findAllByTestId('channel')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Scan for channels' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Scan' }))
    expect(await screen.findByText('a scan is already running')).toBeInTheDocument()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })

  it('shows how far a scan has got, with a way to stop it', async () => {
    const calls = show(doc({ scan: { inProgress: true, possible: false, source: 'Antenna', sources: ['Antenna'], progress: 40, found: 12 } }), { 'DELETE /devices/antenna/channel-scan': () => ({ status: 204 }) })
    await screen.findAllByTestId('channel')
    expect(screen.getByRole('status')).toHaveTextContent('Scanning Antenna: found 12 programs (40%)')
    expect(screen.getByRole('progressbar', { name: 'How far the scan has got' })).toHaveAttribute('aria-valuenow', '40')
    expect(screen.queryByRole('button', { name: 'Scan for channels' })).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Stop' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
  })

  it('cannot be started when the tuner says it cannot scan, or is restarting', async () => {
    const { unmount } = (() => {
      show(doc({ scan: { inProgress: false, possible: false, sources: [] } }))
      return { unmount: () => {} }
    })()
    await screen.findAllByTestId('channel')
    expect(screen.getByRole('button', { name: 'Scan for channels' })).toBeDisabled()
    expect(screen.getByText(/can't scan for channels right now/)).toBeInTheDocument()
    void unmount
  })

  it('keeps asking how far it has got while it runs, and says once when it ends', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      let n = 0
      const running = (k: number) => doc({ scan: { inProgress: true, possible: false, source: 'Antenna', sources: ['Antenna'], progress: 10 * k, found: k } })
      const { fn, calls } = stubFetch({ [path]: () => ({ body: ++n < 3 ? running(n) : doc() }) })
      vi.stubGlobal('fetch', fn)
      renderTunerRoutes(makeState(), '/tuner/antenna/channels')
      await screen.findByRole('status')
      await vi.advanceTimersByTimeAsync(2100)
      await vi.advanceTimersByTimeAsync(2100)
      await vi.advanceTimersByTimeAsync(100)
      expect(await screen.findAllByText('Channel scan finished')).toHaveLength(1)
      expect(calls.filter((c) => c.url.endsWith('/channels')).length).toBeGreaterThanOrEqual(3)
    } finally {
      vi.useRealTimers()
    }
  })
})
