import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { App } from '@/App'
import { Toaster } from '@/components/ui/sonner'
import { AppContext } from '@/hooks/app-context'
import type { DeviceState, RestartStatus } from '@/lib/types'
import { makeDevice, makeState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { DevicePanel } from './device-panel'

afterEach(() => {
  vi.unstubAllGlobals()
  toast.dismiss()
})

type State = ReturnType<typeof makeState>
const path = '/devices/antenna/restart'
const info = { hardwareModel: 'HDFX-4K', readAt: new Date().toISOString() }
const okRoutes = { 'GET /devices/antenna/info': () => ({ body: info }), 'GET /state': () => ({ body: makeState() }) }

const busy = [
  { resource: 'tuner0', inUse: true, viaProxy: true, channel: '5.1' },
  { resource: 'tuner1', inUse: true, viaProxy: false, channel: '2.1' },
]

function withDevice(over: Partial<DeviceState> & { inUse?: number } = {}): State {
  const { inUse = 0, ...rest } = over
  return makeState({
    devices: [makeDevice({ tuner: { ...makeDevice().tuner, slots: busy.slice(0, inUse) }, summary: { total: 4, inUse, remote: Math.min(inUse, 1), local: Math.max(0, inUse - 1), reserved: 1, free: 4 - inUse }, ...rest })],
  })
}

const sec = () => screen.getByRole('region', { name: 'Device' })
const show = (s: State) => renderWithApp(<DevicePanel device={s.devices[0]} />, s)
const running: RestartStatus = { phase: 'restarting', started: '2026-10-18T10:00:00Z' }

describe('Restart', () => {
  it('asks first, and sends nothing until you agree', async () => {
    const { fn, calls } = stubFetch({ [`POST ${path}`]: () => ({ status: 202, body: {} }), ...okRoutes })
    vi.stubGlobal('fetch', fn)
    show(withDevice())
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Restart' }))
    const d = await screen.findByRole('alertdialog')
    expect(within(d).getByText(/Restart .*\?/)).toBeInTheDocument()
    expect(within(d).queryByRole('alert')).not.toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(0)

    await user.click(within(d).getByRole('button', { name: 'Restart' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ interrupt: false }))
  })

  it('says what it will cut off, and sends the interrupt only because you were told', async () => {
    const { fn, calls } = stubFetch({ [`POST ${path}`]: () => ({ status: 202, body: {} }), ...okRoutes })
    vi.stubGlobal('fetch', fn)
    show(withDevice({ inUse: 2 }))
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Restart' }))
    const d = await screen.findByRole('alertdialog')
    expect(within(d).getByRole('alert')).toHaveTextContent('2 of its tuners are in use right now.')
    await user.click(within(d).getByRole('button', { name: 'Interrupt and restart' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ interrupt: true }))
  })

  it('stays open with the reason when the server refuses', async () => {
    vi.stubGlobal('fetch', stubFetch({ [`POST ${path}`]: () => ({ status: 409, body: { error: 'the tuner is offline' } }), ...okRoutes }).fn)
    show(withDevice())
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Restart' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Restart' }))
    expect(await screen.findByText('the tuner is offline')).toBeInTheDocument()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })

  it.each([
    ['the tuner is offline', () => withDevice({ tuner: { ...makeDevice().tuner, online: false } })],
    ['it is being updated', () => withDevice({ tuner: { ...makeDevice().tuner, updating: true } })],
    ['a restart is running', () => withDevice({ restart: running })],
    ['the tuner is restarting', () => withDevice({ tuner: { ...makeDevice().tuner, restarting: true } })],
  ])('cannot be asked for when %s', (_, make) => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    show(make())
    expect(within(sec()).getByRole('button', { name: 'Restart' })).toBeDisabled()
  })
})

describe('what a restart shows', () => {
  it.each([
    [{ ...running }, 'Restarting', 'The tuner is restarting. It will be back on its own.'],
    [{ phase: 'done', started: running.started, finished: '2026-10-18T10:01:00Z' } as RestartStatus, 'Restarted', 'The tuner is back.'],
    [{ phase: 'failed', started: running.started, error: 'the tuner has not come back after 5m0s.' } as RestartStatus, 'The restart failed', 'the tuner has not come back after 5m0s.'],
  ])('%#: %s', (restart, title, text) => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    show(withDevice({ restart }))
    expect(within(sec()).getByText(title)).toBeInTheDocument()
    expect(within(sec()).getByText(text)).toBeInTheDocument()
    expect(!!within(sec()).queryByRole('button', { name: /Dismiss/ })).toBe(restart.phase !== 'restarting')
  })

  it('can dismiss a finished one', async () => {
    const { fn, calls } = stubFetch({ [`DELETE ${path}`]: () => ({ status: 204 }), ...okRoutes })
    vi.stubGlobal('fetch', fn)
    show(withDevice({ restart: { phase: 'done', started: running.started, finished: '2026-10-18T10:01:00Z' } }))
    await userEvent.setup().click(within(sec()).getByRole('button', { name: /Dismiss/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
  })
})

describe('a tuner that is restarting', () => {
  const restarting = () => withDevice({ tuner: { ...makeDevice().tuner, online: false, restarting: true }, restart: running })

  it('is not an outage: no offline banner, and the page says restarting', () => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    renderWithApp(<App />, restarting(), { route: '/' })
    expect(screen.queryByText(/is offline/)).not.toBeInTheDocument()
    expect(screen.getByText('Restarting…')).toBeInTheDocument()
    expect(screen.getByTitle('Restarting')).toHaveClass('animate-pulse')
  })
})

function liveApp(initial: State) {
  let set: (s: State) => void = () => {}
  function Harness() {
    const [state, setState] = useState(initial)
    useEffect(() => {
      set = setState
    }, [])
    return (
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/']}>
          <AppContext.Provider value={{ state, connected: true }}>
            <App />
          </AppContext.Provider>
        </MemoryRouter>
        <Toaster />
      </QueryClientProvider>
    )
  }
  render(<Harness />)
  return (next: State) => act(() => set(next))
}

describe('the toast when a restart ends', () => {
  const stateWith = (restart?: RestartStatus) => withDevice({ restart })

  it('says so once when a restart that was running finishes', async () => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    const update = liveApp(stateWith(running))
    update(stateWith({ phase: 'done', started: running.started, finished: '2026-10-18T10:01:00Z' }))
    expect(await screen.findByText('Steve FLEX 4K restarted')).toBeInTheDocument()
    update(stateWith({ phase: 'done', started: running.started, finished: '2026-10-18T10:01:00Z' }))
    await new Promise((r) => setTimeout(r, 30))
    expect(screen.getAllByText('Steve FLEX 4K restarted')).toHaveLength(1)
  })

  it('says why when it failed', async () => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    const update = liveApp(stateWith(running))
    update(stateWith({ phase: 'failed', started: running.started, error: 'the tuner kept answering, so it does not look as if it restarted.' }))
    expect(await screen.findByText('Restart failed: the tuner kept answering, so it does not look as if it restarted.')).toBeInTheDocument()
  })

  it('does not announce one that had already ended when the page opened', async () => {
    vi.stubGlobal('fetch', stubFetch(okRoutes).fn)
    liveApp(stateWith({ phase: 'done', started: running.started, finished: '2026-10-18T10:01:00Z' }))
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByText(/restarted/)).not.toBeInTheDocument()
  })
})
