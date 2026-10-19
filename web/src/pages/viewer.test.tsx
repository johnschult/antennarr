import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '@/App'
import type { HistoryDoc, HistorySession } from '@/lib/types'
import { makeState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

// Built from local dates, so the days mean the same wherever the tests run.
const local = (y: number, mo: number, d: number, h = 0, mi = 0) => new Date(y, mo - 1, d, h, mi).toISOString()
const now = local(2026, 10, 19, 21, 0)
const GB = 1_000_000_000
/** The link back, not the one in the top bar. */
const backLink = () => screen.getAllByRole('link', { name: /Viewers/ }).find((l) => !l.closest('nav'))!
const s = (over: Partial<HistorySession>): HistorySession => ({
  device: 'antenna', tuner: 'tuner2', channel: '2.1', channelName: 'WSB-HD', kind: 'viewer', viewer: 'John', start: local(2026, 10, 19, 20, 0), end: local(2026, 10, 19, 21, 30), bytes: 2 * GB, quality: 90, ...over,
})
const doc = (sessions: HistorySession[]): HistoryDoc => ({ days: 30, now, sessions })

function show(sessions: HistorySession[], route = '/viewers/John', state = makeState()) {
  const { fn, calls } = stubFetch({
    'GET /history?days=30': () => ({ body: doc(sessions) }),
    'GET /history?days=7': () => ({ body: { ...doc(sessions), days: 7 } }),
  })
  vi.stubGlobal('fetch', fn)
  renderWithApp(<App />, state, { route })
  return calls
}

describe('ViewerPage', () => {
  it('shows only that viewer’s data, time and sessions', async () => {
    show([
      s({}),
      s({ start: local(2026, 10, 18, 19), end: local(2026, 10, 18, 20), channel: '5.1', channelName: 'WFRV', bytes: GB, quality: 50, titles: ['Evening News'] }),
      s({ viewer: 'Mom', bytes: 40 * GB }), // someone else's
      s({ kind: 'local', viewer: undefined, bytes: 40 * GB }), // and the owner's own devices
    ])
    expect(await screen.findByRole('heading', { level: 1, name: 'John' })).toBeInTheDocument()
    await screen.findByText('Data used')
    const stat = (label: string) => screen.getByText(label).parentElement!
    expect(stat('Data used')).toHaveTextContent('3.0 GB')
    expect(stat('Time watched')).toHaveTextContent('2h 30m')
    expect(stat('Time watched')).toHaveTextContent('2 sessions')
    expect(stat('Most data in a day')).toHaveTextContent('2.0 GB')
    expect(screen.getAllByTestId('session')).toHaveLength(2)
    expect(screen.queryByText('40 GB')).not.toBeInTheDocument()
  })

  it('draws the data and the time of each day, and lists what they watched with the reception', async () => {
    show([s({}), s({ start: local(2026, 10, 18, 19), end: local(2026, 10, 18, 20), channel: '5.1', channelName: 'WFRV', bytes: GB, quality: 50 })])
    await screen.findByText('Data sent each day')
    expect(screen.getByText('Time watched each day')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Data sent each day: .*2\.0 GB/ })).toBeInTheDocument()
    const watched = screen.getByRole('region', { name: 'What they watched' })
    const rows = within(watched).getAllByTestId('total')
    expect(rows[0]).toHaveTextContent('2.1 WSB-HD')
    expect(rows[0]).toHaveTextContent('90% quality')
    expect(rows[1]).toHaveTextContent('5.1 WFRV')
    expect(rows[1]).toHaveTextContent('50% quality')
  })

  it('does not repeat who it was on every session, and says how much data each took', async () => {
    show([s({})])
    const row = await screen.findByTestId('session')
    expect(row).toHaveTextContent('1h 30m')
    expect(row).toHaveTextContent('2.0 GB')
    expect(row).not.toHaveTextContent('John')
  })

  it('goes back to the viewers', async () => {
    show([s({})])
    await screen.findByTestId('session')
    expect(backLink()).toHaveAttribute('href', '/viewers')
  })

  it('asks for 7 days when told to', async () => {
    const calls = show([s({})])
    await screen.findByTestId('session')
    expect(screen.getByRole('button', { name: '30 days' })).toHaveAttribute('aria-pressed', 'true')
    await userEvent.setup().click(screen.getByRole('button', { name: '7 days' }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/history?days=7'))).toBe(true))
  })

  it('says so when they have not watched anything', async () => {
    show([s({ viewer: 'Mom' })])
    expect(await screen.findByText(/John hasn't watched anything in the last 30 days/)).toBeInTheDocument()
    expect(screen.queryByText('Data used')).not.toBeInTheDocument()
  })

  it('says there is no such viewer, and asks for nothing', async () => {
    const calls = show([s({})], '/viewers/Nobody')
    expect(await screen.findByRole('heading', { level: 1 })).toHaveTextContent('No viewer called Nobody')
    expect(backLink()).toHaveAttribute('href', '/viewers')
    await new Promise((r) => setTimeout(r, 30))
    expect(calls.filter((c) => c.url.includes('/history'))).toHaveLength(0)
  })

  it('finds a viewer whose name needs encoding', async () => {
    const state = makeState()
    state.viewers = [{ ...state.viewers[0], name: 'Mom & Dad' }]
    show([s({ viewer: 'Mom & Dad' })], '/viewers/Mom%20%26%20Dad', state)
    expect(await screen.findByRole('heading', { level: 1, name: 'Mom & Dad' })).toBeInTheDocument()
    expect(await screen.findByTestId('session')).toBeInTheDocument()
  })

  it('says the history is off, with a way to turn it on, and asks for nothing', async () => {
    const state = makeState()
    state.settings.history = false
    const calls = show([s({})], '/viewers/John', state)
    expect(await screen.findByText(/The activity history is off/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Turn it on in Settings' })).toHaveAttribute('href', '/settings/privacy')
    await new Promise((r) => setTimeout(r, 30))
    expect(calls.filter((c) => c.url.includes('/history'))).toHaveLength(0)
  })
})

describe('getting to a viewer’s page', () => {
  it('from the Viewers page, by their name, while the history is kept', async () => {
    show([], '/viewers')
    const link = await screen.findByRole('link', { name: 'John' })
    expect(link).toHaveAttribute('href', '/viewers/John')
  })

  it('but not when it is off, where there would be nothing to see', async () => {
    const state = makeState()
    state.settings.history = false
    show([], '/viewers', state)
    await screen.findByText('John')
    expect(screen.queryByRole('link', { name: 'John' })).not.toBeInTheDocument()
  })

  it('from the Activity page, for a viewer but not for your own devices', async () => {
    show([s({}), s({ kind: 'local', viewer: undefined, client: '192.168.1.5', bytes: GB })], '/activity')
    const byViewer = await screen.findByRole('region', { name: 'By viewer' })
    expect(within(byViewer).getByRole('link', { name: 'John' })).toHaveAttribute('href', '/viewers/John')
    expect(within(byViewer).getByText('Your devices').closest('a')).toBeNull()
    expect(within(byViewer).getByText(/your network/)).toBeInTheDocument()
  })
})
