import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import { useEffect, useState } from 'react'
import { MemoryRouter } from 'react-router'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '@/App'
import type { DeviceState, UpgradeStatus } from '@/lib/types'
import { Toaster } from '@/components/ui/sonner'
import { AppContext } from '@/hooks/app-context'
import { makeDevice, makeState, updateAvailable } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { FirmwareSection } from './firmware'

afterEach(() => {
  vi.unstubAllGlobals()
  toast.dismiss()
})

const path = '/devices/antenna/firmware/upgrade'
type State = ReturnType<typeof makeState>

const idle = [{ resource: 'tuner0', inUse: false, viaProxy: false }]
const busy = [
  { resource: 'tuner0', inUse: true, viaProxy: true, channel: '5.1' },
  { resource: 'tuner1', inUse: true, viaProxy: false, channel: '2.1' },
]

/** A tuner with newer firmware on offer, and (optionally) tuners in use or an install in some phase. */
function withUpdate(over: Partial<DeviceState> & { inUse?: number; upgrade?: UpgradeStatus } = {}): State {
  const { inUse = 0, upgrade, ...rest } = over
  const device = makeDevice({
    firmware: { ...updateAvailable, current: '20250815', latest: '20260326' },
    tuner: { ...makeDevice().tuner, slots: inUse > 0 ? busy.slice(0, inUse) : idle },
    summary: { total: 4, inUse, remote: Math.min(inUse, 1), local: Math.max(0, inUse - 1), reserved: 1, free: 4 - inUse },
    upgrade,
    ...rest,
  })
  return makeState({ devices: [device] })
}

const sec = () => screen.getByRole('region', { name: 'Firmware' })
const show = (s: State) => renderWithApp(<FirmwareSection device={s.devices[0]} />, s)
const okState = { 'GET /state': () => ({ body: makeState() }) }

const running = (phase: UpgradeStatus['phase'], percent = 0): UpgradeStatus => ({ phase, release: '20260326', from: '20250815', percent, started: '2026-10-18T10:00:00Z' })

describe('Install firmware', () => {
  it('is offered when a newer release applies and the tuner is online, with the release named', () => {
    show(withUpdate())
    expect(within(sec()).getByRole('button', { name: 'Install' })).toBeEnabled()
  })

  it.each([
    ['it is up to date', () => makeState()],
    ['only a possible update', () => withUpdate({ firmware: { ...updateAvailable, state: 'possible-update' } })],
    ['the tuner is offline', () => withUpdate({ tuner: { ...makeDevice().tuner, online: false } })],
    ['the tuner is already updating', () => withUpdate({ tuner: { ...makeDevice().tuner, updating: true } })],
    ['an install is running', () => withUpdate({ upgrade: running('sending', 40) })],
  ])('is not offered when %s', (_, make) => {
    show(make())
    expect(within(sec()).queryByRole('button', { name: /Install/ })).not.toBeInTheDocument()
  })

  it('asks first, saying what will happen, and sends nothing until you agree', async () => {
    const { fn, calls } = stubFetch({ [`POST ${path}`]: () => ({ status: 202, body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    show(withUpdate())
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Install' }))

    const d = await screen.findByRole('alertdialog')
    expect(within(d).getByText('Update firmware to 26 Mar 2026?')).toBeInTheDocument()
    expect(within(d).getByText(/download this release from SiliconDust/)).toBeInTheDocument()
    expect(within(d).getByText(/two to three minutes/)).toBeInTheDocument()
    expect(within(d).getByText(/Don't unplug it/)).toBeInTheDocument()
    expect(within(d).getByText(/download\.silicondust\.com over HTTPS/)).toBeInTheDocument()
    expect(within(d).getByText(/can't be undone from here/)).toBeInTheDocument()
    expect(within(d).queryByRole('alert')).not.toBeInTheDocument() // nothing is running, so nothing to warn about
    expect(calls).toEqual([])

    await user.click(within(d).getByRole('button', { name: 'Update' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ version: '20260326', interrupt: false })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('warns what it will cut off, and only then sends the agreement to interrupt', async () => {
    const { fn, calls } = stubFetch({ [`POST ${path}`]: () => ({ status: 202, body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    show(withUpdate({ inUse: 2 }))
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Install' }))
    const d = await screen.findByRole('alertdialog')
    const warning = within(d).getByRole('alert')
    expect(warning).toHaveTextContent('2 of its tuners are in use right now')
    expect(warning).toHaveTextContent(/viewers, your own devices or recordings/)
    expect(within(d).queryByRole('button', { name: 'Update' })).not.toBeInTheDocument()

    await user.click(within(d).getByRole('button', { name: 'Interrupt and update' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ version: '20260326', interrupt: true })
  })

  it('says "tuner" and "is" for one that is in use', async () => {
    show(withUpdate({ inUse: 1 }))
    await userEvent.setup().click(within(sec()).getByRole('button', { name: 'Install' }))
    expect(within(await screen.findByRole('alertdialog')).getByRole('alert')).toHaveTextContent('1 of its tuner is in use right now')
  })

  it('does nothing when you say not now', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    show(withUpdate())
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Install' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Not now/ }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(calls).toEqual([])
  })

  it('says why when the server refuses, and leaves the question open', async () => {
    vi.stubGlobal('fetch', stubFetch({ [`POST ${path}`]: () => ({ status: 409, body: { error: 'the newest firmware is 20260401 now, not "20260326": look again before updating' } }) }).fn)
    show(withUpdate())
    const user = userEvent.setup()
    await user.click(within(sec()).getByRole('button', { name: 'Install' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Update' }))
    expect(await screen.findByText(/look again before updating/)).toBeInTheDocument()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })
})

describe('what an install shows', () => {
  it.each([
    ['downloading', 'Downloading 26 Mar 2026 from SiliconDust…'],
    ['sending', 'Sending it to the tuner… 40%'],
    ['restarting', "The tuner is installing it and restarting. This takes a couple of minutes: don't unplug it."],
    ['verifying', 'The tuner is back. Checking what it is running…'],
  ] as const)('says what it is doing while %s, with no way to start another', (phase, text) => {
    show(withUpdate({ upgrade: running(phase, 40) }))
    const row = within(sec())
    expect(row.getByRole('status')).toHaveTextContent(text)
    expect(row.getByText('Updating')).toBeInTheDocument()
    expect(row.queryByRole('button', { name: /Install|Dismiss/ })).not.toBeInTheDocument()
  })

  it('says it is done, and can be dismissed', async () => {
    const { fn, calls } = stubFetch({ [`DELETE ${path}`]: () => ({ status: 204 }), ...okState })
    vi.stubGlobal('fetch', fn)
    show(withUpdate({ upgrade: { ...running('done', 100), finished: '2026-10-18T10:03:00Z' } }))
    expect(within(sec()).getByText('Updated from 15 Aug 2025 to 26 Mar 2026.')).toBeInTheDocument()
    await userEvent.setup().click(within(sec()).getByRole('button', { name: /Dismiss/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
  })

  it('shows why a failed one failed, in red, and can be dismissed', () => {
    show(withUpdate({ upgrade: { ...running('failed', 100), error: 'the tuner refused the firmware (ERROR). It is still running 20250815.' } }))
    expect(within(sec()).getByText('The update failed')).toBeInTheDocument()
    expect(within(sec()).getByText(/the tuner refused the firmware/)).toHaveClass('text-red-700')
    expect(within(sec()).getByRole('button', { name: /Dismiss/ })).toBeInTheDocument()
  })
})

describe('a tuner that is updating', () => {
  const updating = (): State => withUpdate({ tuner: { ...makeDevice().tuner, online: false, updating: true }, upgrade: running('restarting', 100) })

  it('is not an outage: no offline banner, and the page says updating', () => {
    renderWithApp(<App />, updating(), { route: '/' })
    expect(screen.queryByText(/is offline/)).not.toBeInTheDocument()
    expect(screen.getByText('Updating firmware…')).toBeInTheDocument()
    expect(screen.getByTitle('Updating firmware')).toHaveClass('animate-pulse')
  })

  it('shows it on the Tuners page too', () => {
    renderWithApp(<App />, updating(), { route: '/tuners' })
    expect(screen.getByTitle('Updating firmware')).toBeInTheDocument()
  })

  it('is still an outage when the tuner is offline and not updating', () => {
    renderWithApp(<App />, makeState({ devices: [makeDevice({ tuner: { ...makeDevice().tuner, online: false } })] }), { route: '/' })
    expect(screen.getByText(/The tuner is offline/)).toBeInTheDocument()
    expect(screen.queryByText('Updating firmware…')).not.toBeInTheDocument()
  })
})

/** The whole app on a state that a test can replace, as the event stream replaces it. */
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

describe('the toast when an install ends', () => {
  const stateWith = (u: UpgradeStatus | undefined) => withUpdate({ upgrade: u })

  it('says so once when an install that was running finishes, wherever you are', async () => {
    const update = liveApp(stateWith(running('sending', 50)))
    expect(screen.queryByText(/Updated/)).not.toBeInTheDocument()
    update(stateWith({ ...running('done', 100), finished: '2026-10-18T10:03:00Z' }))
    expect(await screen.findByText('Updated Steve FLEX 4K to 26 Mar 2026')).toBeInTheDocument()
    update(stateWith({ ...running('done', 100), finished: '2026-10-18T10:03:00Z' })) // another state update
    await new Promise((r) => setTimeout(r, 30))
    expect(screen.getAllByText('Updated Steve FLEX 4K to 26 Mar 2026')).toHaveLength(1)
  })

  it('says why when it failed', async () => {
    const update = liveApp(stateWith(running('restarting', 100)))
    update(stateWith({ ...running('failed', 100), error: 'the tuner has not come back after 5m0s.' }))
    expect(await screen.findByText('Firmware update failed: the tuner has not come back after 5m0s.')).toBeInTheDocument()
  })

  it('does not announce one that had already ended when the page opened', async () => {
    liveApp(stateWith({ ...running('done', 100), finished: '2026-10-18T10:03:00Z' }))
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByText(/Updated Steve/)).not.toBeInTheDocument()
  })

  it('announces a second install that follows the first', async () => {
    const first = { ...running('done', 100), started: '2026-10-18T10:00:00Z' }
    const second = { ...running('sending', 10), started: '2026-10-18T11:00:00Z' }
    const update = liveApp(stateWith(running('sending', 50)))
    update(stateWith(first))
    expect(await screen.findByText(/Updated Steve/)).toBeInTheDocument()
    toast.dismiss()
    update(stateWith(second))
    update(stateWith({ ...second, phase: 'failed', error: 'x' }))
    expect(await screen.findByText('Firmware update failed: x')).toBeInTheDocument()
  })
})
