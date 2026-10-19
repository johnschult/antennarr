import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { channelIconUrl } from '@/lib/api'
import { moveToPort } from '@/lib/navigate'
import { idleSlot, localSlot, makeDevice, makeState, remoteSlot, updateAvailable } from '@/test/fixtures'
import { renderTunerRoutes, stubFetch } from '@/test/render'

vi.mock('@/lib/navigate', () => ({ moveToPort: vi.fn() }))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.mocked(moveToPort).mockClear()
})

const okState = { 'GET /state': () => ({ body: makeState() }) }
const info = { 'GET /devices/antenna/info': () => ({ body: { hardwareModel: 'HDFX-4K', version: '20260326', readAt: new Date().toISOString() } }) }
const overview = (s = makeState()) => renderTunerRoutes(s, '/tuner/antenna')
const settings = (s = makeState()) => renderTunerRoutes(s, '/tuner/antenna/settings')
const section = (name: string) => screen.getByRole('region', { name })
const saveButton = (scope: HTMLElement) => within(scope).queryByRole('button', { name: 'Save' })

describe('a tuner’s page', () => {
  it('has the tuner’s name, what it is, where it is and how many of its tuners are in use', () => {
    overview(makeState({ devices: [makeDevice({ label: 'Living room antenna' })] }))
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Living room antenna')
    const subtitle = screen.getByRole('heading', { level: 1 }).nextElementSibling!
    expect(subtitle).toHaveTextContent('Steve FLEX 4K · 192.168.1.132')
    expect(subtitle).not.toHaveTextContent('HDFX-4K') // the model number is for the Device tab
    expect(screen.getByText(/\d of 4 in use/)).toBeInTheDocument()
  })

  it('does not say the device twice when it has no name of your own', () => {
    overview()
    const subtitle = screen.getByRole('heading', { level: 1 }).nextElementSibling!
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Steve FLEX 4K')
    expect(subtitle).not.toHaveTextContent('Steve FLEX 4K')
  })

  it('goes back to the list of tuners, which a single tuner would otherwise skip', () => {
    overview()
    const back = screen.getAllByRole('link', { name: /Tuners/ }).find((l) => !l.closest('nav'))!
    expect(back).toHaveAttribute('href', '/tuner?list')
  })

  it('says there is no such tuner, with a way back', () => {
    renderTunerRoutes(makeState(), '/tuner/nobody')
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('No tuner called nobody')
    expect(screen.getByRole('link', { name: /Tuners/ })).toHaveAttribute('href', '/tuner?list')
  })

  it('sends an unknown tab to the overview', () => {
    renderTunerRoutes(makeState(), '/tuner/antenna/nonsense')
    expect(screen.getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
  })

  it('has five sections as links with their own addresses, and marks the one you are on', () => {
    overview()
    const nav = screen.getByRole('navigation', { name: 'Sections of this tuner' })
    const links = within(nav).getAllByRole('link')
    expect(links.map((l) => [l.textContent, l.getAttribute('href')])).toEqual([
      ['Overview', '/tuner/antenna'],
      ['Channels', '/tuner/antenna/channels'],
      ['Device', '/tuner/antenna/device'],
      ['Firmware', '/tuner/antenna/firmware'],
      ['Settings', '/tuner/antenna/settings'],
    ])
    expect(within(nav).getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
    expect(within(nav).getByRole('link', { name: 'Device' })).not.toHaveAttribute('aria-current')
  })

  it('shows one section at a time', async () => {
    vi.stubGlobal('fetch', stubFetch({ ...info, ...okState }).fn)
    overview()
    expect(screen.getByText('Last seen')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Device' })).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('link', { name: 'Device' }))
    expect(await screen.findByRole('region', { name: 'Device' })).toBeInTheDocument()
    expect(screen.queryByText('Last seen')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Device' })).toHaveAttribute('aria-current', 'page')
  })
})

describe('the overview', () => {
  it('shows what we know about the tuner, and links what has details on another tab', () => {
    overview()
    const facts = screen.getByText('DeviceID').closest('dl')!
    for (const text of ['Steve FLEX 4K', 'HDFX-4K', '10ABCC87', '192.168.1.132']) {
      expect(within(facts).getByText(text)).toBeInTheDocument()
    }
    // how it is now comes first, above the tuners and the details
    const statusEl = screen.getByTestId('status')
    const status = within(statusEl)
    expect(status.getByText('Online')).toBeInTheDocument()
    expect(status.getByText('4')).toBeInTheDocument()
    expect(status.getByText(/Last seen/)).toBeInTheDocument()
    expect(statusEl.compareDocumentPosition(facts as Node) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // the device's details are on the Device tab, and its name and port are settings
    for (const text of ['Steve FLEX 4K', 'HDFX-4K', '10ABCC87', '192.168.1.132']) {
      expect(within(facts).getByRole('link', { name: text })).toHaveAttribute('href', '/tuner/antenna/device')
    }
    expect(within(facts).getByRole('link', { name: '5004' })).toHaveAttribute('href', '/tuner/antenna/settings')
    // how many tuners it has and when it was last seen have nowhere else to go
    expect(status.queryByRole('link', { name: '4' })).not.toBeInTheDocument()
    expect(status.queryByRole('link', { name: /ago|just now/ })).not.toBeInTheDocument()
  })

  it('has a Name only when you gave one, and links it to Settings, where it is changed', () => {
    const first = overview()
    expect(screen.queryByText('Name')).not.toBeInTheDocument() // the tuner's own name is the Device below
    first.unmount()
    overview(makeState({ devices: [makeDevice({ label: 'Living room antenna' })] }))
    const facts = screen.getByText('DeviceID').closest('dl')!
    expect(within(facts).getByRole('link', { name: 'Living room antenna' })).toHaveAttribute('href', '/tuner/antenna/settings')
  })

  it('lists each tuner and who is using it', () => {
    const s = makeState()
    s.devices[0].tuner.slots = [{ ...localSlot(0), signalStrength: 90, signalQuality: 100 }, { ...remoteSlot(1), signalStrength: 85, signalQuality: 95 }, idleSlot(2)]
    overview(s)
    const rows = within(screen.getByRole('list', { name: 'Tuners' })).getAllByRole('listitem')
    expect(rows).toHaveLength(3)
    expect(rows[0]).toHaveTextContent('tuner0')
    expect(rows[0]).toHaveTextContent('2.1 WBAY')
    expect(rows[0]).toHaveTextContent('192.168.1.77')
    expect(rows[0]).toHaveTextContent('90% / 100%')
    expect(rows[1]).toHaveTextContent('Antennarr')
    expect(rows[1]).toHaveTextContent('14 Mbps')
    expect(rows[2]).toHaveTextContent('idle')
  })

  it('points an offline tuner at the logs for it', () => {
    const s = makeState()
    s.devices[0].tuner = { ...s.devices[0].tuner, online: false }
    overview(s)
    expect(screen.getByRole('link', { name: 'See why in the logs' })).toHaveAttribute('href', '/settings/logs?q=antenna')
  })

  it('has no pointer to the logs while it is online', () => {
    overview()
    expect(screen.queryByRole('link', { name: 'See why in the logs' })).not.toBeInTheDocument()
  })

  it('copes with a tuner that has never been seen', () => {
    const s = makeState()
    s.devices[0].tuner = { ...s.devices[0].tuner, online: false, id: '', friendlyName: undefined, model: undefined, slots: [] }
    overview(s)
    expect(screen.getByText('Not yet seen')).toBeInTheDocument()
    expect(screen.getByText('Offline')).toBeInTheDocument()
    expect(screen.getByText('No tuner data yet.')).toBeInTheDocument()
  })

  it('shows the logo beside each watched channel that has one, and none otherwise', () => {
    const s = makeState()
    s.devices[0].guide = { '5.1': { name: 'WFRV', icon: true }, '2.1': { name: 'WBAY', icon: false } }
    overview(s)
    const logos = screen.getAllByRole('img', { name: /logo/ })
    expect(logos).toHaveLength(1) // 5.1 only: 2.1 has no logo, the idle tuners have no channel
    expect(logos[0]).toHaveAttribute('src', channelIconUrl('antenna', '5.1'))
    expect(logos[0].closest('li')).toHaveTextContent('5.1')
  })

  describe('the firmware line', () => {
    const line = () => screen.getByTestId('firmware-line')

    it('says it is up to date, with a way to the Firmware tab', () => {
      overview()
      expect(line()).toHaveTextContent('26 Mar 2026')
      expect(line()).toHaveTextContent(/Up to date|Not known/)
      expect(within(line()).getByRole('link')).toHaveAttribute('href', '/tuner/antenna/firmware')
    })

    it('says an update is available, and links to what is new', () => {
      overview(makeState({ devices: [makeDevice({ firmware: { ...updateAvailable, current: '20250815', latest: '20260326' } })] }))
      expect(line()).toHaveTextContent('Update available: 26 Mar 2026')
      expect(within(line()).getByRole('link', { name: /See what's new/ })).toHaveAttribute('href', '/tuner/antenna/firmware')
    })

    it('says when the check is off', () => {
      overview(makeState({ devices: [makeDevice({ firmware: { ...makeDevice().firmware, state: 'disabled' } })] }))
      expect(line()).toHaveTextContent('The check is off')
    })
  })

  it('sends a tuner being restarted to its Device tab, and one being updated to Firmware', () => {
    const s = makeState()
    s.devices[0].tuner = { ...s.devices[0].tuner, restarting: true }
    const { unmount } = overview(s)
    expect(screen.getByRole('link', { name: 'Restarting…' })).toHaveAttribute('href', '/tuner/antenna/device')
    unmount()
    const u = makeState()
    u.devices[0].tuner = { ...u.devices[0].tuner, updating: true }
    overview(u)
    expect(screen.getByRole('link', { name: 'Updating firmware…' })).toHaveAttribute('href', '/tuner/antenna/firmware')
  })
})

describe('the other tabs', () => {
  it('has the Device panel, and a way to the tuner’s own page for what Antennarr does not show', async () => {
    vi.stubGlobal('fetch', stubFetch({ ...info, ...okState }).fn)
    renderTunerRoutes(makeState(), '/tuner/antenna/device')
    expect(await screen.findByRole('region', { name: 'Device' })).toBeInTheDocument()
    const own = screen.getByRole('link', { name: /Open the tuner's own page/ })
    expect(own).toHaveAttribute('href', 'http://192.168.1.132')
    expect(own).toHaveAttribute('rel', expect.stringContaining('noopener'))
  })

  it('has the Firmware section, without naming the tuner a page that is already about it', () => {
    renderTunerRoutes(makeState(), '/tuner/antenna/firmware')
    expect(screen.getByRole('region', { name: 'Firmware' })).toBeInTheDocument()
  })

  it('has the Settings section', () => {
    settings()
    expect(section('Settings')).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    expect(screen.getByLabelText('Port')).toBeInTheDocument()
  })
})

describe('naming a tuner', () => {
  it('saves the name, trimmed, on that tuner', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    settings()

    expect(saveButton(section('Settings'))).toBeNull()
    await user.type(screen.getByLabelText('Name'), '  Antenna (Steve)  ')
    await user.click(saveButton(section('Settings'))!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
    expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/antenna'), body: { label: 'Antenna (Steve)' } })
    expect(await screen.findByText('Settings saved')).toBeInTheDocument()
  })

  it('saves with Enter', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    settings()
    await userEvent.setup().type(screen.getByLabelText('Name'), 'Antenna{Enter}')
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: 'Antenna' }))
  })

  it('can be cleared to go back to the tuner’s own name', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    const s = makeState()
    s.devices[0].label = 'Antenna (Steve)'
    settings(s)
    expect(screen.getByLabelText('Name')).toHaveValue('Antenna (Steve)')
    await user.clear(screen.getByLabelText('Name'))
    await user.click(saveButton(section('Settings'))!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: '' }))
  })

  it('stops at 60 characters, the most the server accepts', () => {
    settings()
    expect(screen.getByLabelText('Name')).toHaveAttribute('maxlength', '60')
  })

  it("shows the server's objection on the row", async () => {
    vi.stubGlobal('fetch', stubFetch({ 'PATCH /devices/antenna': () => ({ status: 400, body: { error: 'name for "antenna": can\'t contain control characters' } }) }).fn)
    const user = userEvent.setup()
    settings()
    await user.type(screen.getByLabelText('Name'), 'x')
    await user.click(saveButton(section('Settings'))!)
    expect(await within(section('Settings')).findByRole('alert')).toHaveTextContent('control characters')
  })
})

describe('the port', () => {
  it('shows the port number, and Remove is red', () => {
    settings()
    expect(screen.getByLabelText('Port')).toHaveValue(5004)
    expect(screen.getByRole('button', { name: /^Remove / }).className).toContain('text-destructive')
  })

  it('saves a new port on that tuner, then moves the page there', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    settings()
    const box = screen.getByLabelText('Port')
    await user.clear(box)
    await user.type(box, '5010')
    await user.click(saveButton(section('Settings'))!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
    expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/antenna'), body: { listen: ':5010' } })
    await waitFor(() => expect(moveToPort).toHaveBeenCalledWith(5010))
  })

  it('keeps the address part of where it listens', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    const s = makeState()
    s.devices[0].listen = '127.0.0.1:5004'
    const user = userEvent.setup()
    settings(s)
    await user.clear(screen.getByLabelText('Port'))
    await user.type(screen.getByLabelText('Port'), '5010')
    await user.click(saveButton(section('Settings'))!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ listen: '127.0.0.1:5010' }))
  })

  it('rejects nonsense locally, and stays put when the server refuses', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ status: 409, body: { error: "can't listen on :80: in use" } }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    settings()
    const box = screen.getByLabelText('Port')
    await user.clear(box)
    await user.type(box, '70000')
    await user.click(saveButton(section('Settings'))!)
    expect(within(section('Settings')).getByText('Enter a port from 1 to 65535.')).toBeInTheDocument()
    expect(calls.filter((c) => c.method !== 'GET')).toHaveLength(0)

    await user.clear(box)
    await user.type(box, '80')
    await user.click(saveButton(section('Settings'))!)
    expect(await within(section('Settings')).findByRole('alert')).toHaveTextContent("can't listen on :80: in use")
    expect(moveToPort).not.toHaveBeenCalled()
  })
})
