import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { HistoryDoc, HistorySession } from '@/lib/types'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { ActivityPage } from './activity'

afterEach(() => vi.unstubAllGlobals())

// Built from local dates, so the days mean the same wherever the tests run.
const local = (y: number, mo: number, d: number, h = 0, mi = 0) => new Date(y, mo - 1, d, h, mi).toISOString()
const now = local(2026, 10, 19, 21, 0)
const s = (over: Partial<HistorySession>): HistorySession => ({
  device: 'antenna', tuner: 'tuner2', channel: '2.1', channelName: 'WSB-HD', kind: 'viewer', viewer: 'Alex', start: local(2026, 10, 19, 20, 0), end: local(2026, 10, 19, 21, 30), ...over,
})
const doc = (sessions: HistorySession[], days = 7): HistoryDoc => ({ days, now, sessions })
const show = (body: HistoryDoc | { status: number; error: string }, state = makeState()) => {
  const { fn, calls } = stubFetch({
    'GET /history?days=7': () => ('status' in body ? { status: body.status, body: { error: body.error } } : { body }),
    'GET /history?days=30': () => ({ body: { ...(body as HistoryDoc), days: 30 } }),
  })
  vi.stubGlobal('fetch', fn)
  renderWithApp(<ActivityPage />, state)
  return calls
}

describe('ActivityPage', () => {
  it('lists what was watched, by day, newest first, with who and which tuner', async () => {
    show(
      doc([
        s({ start: local(2026, 10, 18, 19), end: local(2026, 10, 18, 20), channel: '5.1', channelName: 'WFRV', viewer: 'Sam', titles: ['Evening News'] }),
        s({ titles: ['College Football', 'Post-game'] }),
        s({ kind: 'local', viewer: undefined, client: '192.168.68.115', tuner: 'tuner3', channel: '69.1', channelName: 'WUPA', start: local(2026, 10, 19, 8), end: local(2026, 10, 19, 8, 45) }),
      ]),
    )
    const today = await screen.findByRole('region', { name: 'Today' })
    const rows = within(today).getAllByTestId('session')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('2.1 WSB-HD')
    expect(rows[0]).toHaveTextContent('College Football · Post-game')
    expect(rows[0]).toHaveTextContent('1h 30m')
    expect(rows[0]).toHaveTextContent('Alex')
    expect(rows[0]).toHaveTextContent('Tuner 3')
    expect(rows[1]).toHaveTextContent('69.1 WUPA')
    expect(rows[1]).toHaveTextContent('Local')
    const yesterday = screen.getByRole('region', { name: 'Yesterday' })
    expect(within(yesterday).getByTestId('session')).toHaveTextContent('Sam')
  })

  it('says what a session in progress is', async () => {
    show(doc([s({ open: true, end: now })]))
    const row = await screen.findByTestId('session')
    expect(row).toHaveTextContent('– now')
    expect(row).toHaveTextContent('watching')
  })

  it('totals it up at the top', async () => {
    show(
      doc([
        s({}),
        s({ start: local(2026, 10, 19, 10), end: local(2026, 10, 19, 10, 30), channel: '5.1', channelName: 'WFRV', kind: 'local', viewer: undefined }),
      ]),
    )
    await screen.findByText('Time watched')
    expect(screen.getByText('2h 00m')).toBeInTheDocument()
    expect(screen.getByText('2 sessions')).toBeInTheDocument()
    const stat = (label: string) => screen.getByText(label).parentElement!
    expect(stat('Most watched channel')).toHaveTextContent('2.1 WSB-HD')
    expect(stat('Most active')).toHaveTextContent('Alex')
    expect(stat('Busiest day')).toHaveTextContent('2h 00m')
  })

  it('draws a bar for each day, today last', async () => {
    show(doc([s({})]))
    await screen.findByText('Time watched each day')
    const bars = screen.getAllByTestId('day-bar')
    expect(bars).toHaveLength(7)
    expect(bars[6]).toHaveAttribute('data-minutes', '90')
    expect(bars[0]).toHaveAttribute('data-minutes', '0')
    expect(screen.getByRole('img', { name: /Time watched each day: .*1h 30m/ })).toBeInTheDocument()
  })

  it('says so when there is nothing', async () => {
    show(doc([]))
    expect(await screen.findByText(/Nothing watched in the last 7 days/)).toBeInTheDocument()
    expect(screen.queryByTestId('day-bar')).not.toBeInTheDocument()
  })

  it('says so when it could not be loaded', async () => {
    show({ status: 500, error: 'boom' })
    expect(await screen.findByRole('alert')).toHaveTextContent('boom')
  })

  it('asks for 30 days when told to', async () => {
    const calls = show(doc([s({})]))
    await screen.findByTestId('session')
    expect(screen.getByRole('button', { name: '7 days' })).toHaveAttribute('aria-pressed', 'true')
    await userEvent.setup().click(screen.getByRole('button', { name: '30 days' }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/history?days=30'))).toBe(true))
    expect(screen.getByRole('button', { name: '30 days' })).toHaveAttribute('aria-pressed', 'true')
    await waitFor(() => expect(screen.getAllByTestId('day-bar')).toHaveLength(30))
  })

  it('names the tuner box too when there are several', async () => {
    const state = makeTwoDeviceState()
    show(doc([s({ device: state.devices[1].name })]), state)
    const row = await screen.findByTestId('session')
    expect(row).toHaveTextContent(/Cable HDHR · Tuner 3/)
  })

  it('says it is off, links to Settings, and asks for nothing, when the history is off', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    const state = makeState()
    state.settings.history = false
    renderWithApp(<ActivityPage />, state)
    expect(screen.getByText(/The activity history is off/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Turn it on in Settings' })).toHaveAttribute('href', '/settings')
    expect(screen.queryByRole('button', { name: '7 days' })).not.toBeInTheDocument()
    await new Promise((r) => setTimeout(r, 30))
    expect(calls).toHaveLength(0)
  })

  it('warns that it is only in memory when there is no settings file', async () => {
    show(doc([s({})]), makeState({ editable: false }))
    expect(await screen.findByText(/kept in memory and starts over/)).toBeInTheDocument()
  })
})
