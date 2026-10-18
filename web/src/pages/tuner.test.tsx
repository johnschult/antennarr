import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { moveToPort } from '@/lib/navigate'
import { idleSlot, localSlot, makeState, makeTwoDeviceState, remoteSlot, updateAvailable } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { channelIconUrl } from '@/lib/api'
import { TunerPage } from './tuner'

vi.mock('@/lib/navigate', () => ({ moveToPort: vi.fn() }))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.mocked(moveToPort).mockClear()
})

const section = (name: string) => screen.getByRole('region', { name })
const saveButton = (scope: HTMLElement) => within(scope).queryByRole('button', { name: 'Save' })
const okState = { 'GET /state': () => ({ body: makeState() }) }

describe('TunerPage', () => {
  it('shows what we know about the device', () => {
    renderWithApp(<TunerPage />, makeState())
    const facts = screen.getByText('DeviceID').closest('dl')!
    for (const text of ['Steve FLEX 4K', 'HDFX-4K', '10ABCC87', '192.168.1.132', '4']) {
      expect(within(facts).getByText(text)).toBeInTheDocument()
    }
    expect(within(facts).getByText('Online')).toBeInTheDocument()
  })

  it('lists each tuner and who is using it', () => {
    const s = makeState()
    s.devices[0].tuner.slots = [
      { ...localSlot(0), signalStrength: 90, signalQuality: 100 },
      { ...remoteSlot(1), signalStrength: 85, signalQuality: 95 },
      idleSlot(2),
    ]
    renderWithApp(<TunerPage />, s)
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

  it('copes with a tuner that has never been seen', () => {
    const s = makeState()
    s.devices[0].tuner = { ...s.devices[0].tuner, online: false, id: '', friendlyName: undefined, model: undefined, slots: [] }
    renderWithApp(<TunerPage />, s)
    expect(screen.getByText('Not yet seen')).toBeInTheDocument()
    expect(screen.getByText('Offline')).toBeInTheDocument()
    expect(screen.getByText('No tuner data yet.')).toBeInTheDocument()
  })

  it('has a section for each device when there are several, folded until opened', async () => {
    const user = userEvent.setup()
    renderWithApp(<TunerPage />, makeTwoDeviceState())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
    const antenna = screen.getByRole('region', { name: 'Steve FLEX 4K' })
    const cable = screen.getByRole('region', { name: 'Cable HDHR' })
    expect(within(antenna).getByRole('button', { name: /^Steve FLEX 4K/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('10ABCC87')).not.toBeInTheDocument()
    expect(antenna).toHaveTextContent('HDFX-4K')
    expect(antenna).toHaveTextContent('port 5004')
    expect(antenna).toHaveTextContent('2 of 4 in use')
    expect(cable).toHaveTextContent('0 of 2 in use')

    await user.click(within(antenna).getByRole('button', { name: /^Steve FLEX 4K/ }))
    await user.click(within(cable).getByRole('button', { name: /^Cable HDHR/ }))
    expect(within(antenna).getByText('10ABCC87')).toBeInTheDocument()
    expect(within(cable).getByText('1111AAA2')).toBeInTheDocument()
    expect(within(cable).getByText('HDHR5-4DT')).toBeInTheDocument()
    expect(within(antenna).getAllByRole('listitem').length).toBeGreaterThanOrEqual(4)
    expect(within(cable).getByRole('list', { name: 'Tuners on Cable HDHR' })).toBeInTheDocument()

    await user.click(within(antenna).getByRole('button', { name: /^Steve FLEX 4K/ }))
    expect(screen.queryByText('10ABCC87')).not.toBeInTheDocument()
  })

  it('opens a single tuner by itself', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.getByRole('button', { name: /^Steve FLEX 4K/ })).toHaveAttribute('aria-expanded', 'true')
  })

  it('shows how many tuners are in use and a firmware flag while folded', () => {
    const s = makeTwoDeviceState()
    s.devices[0].tuner.slots = [localSlot(0), remoteSlot(1), idleSlot(2), idleSlot(3)]
    s.devices[0].firmware = updateAvailable
    renderWithApp(<TunerPage />, s)
    const antenna = screen.getByRole('region', { name: 'Steve FLEX 4K' })
    expect(antenna).toHaveTextContent('2 of 4 in use')
    expect(within(antenna).getByText('Firmware update')).toBeInTheDocument()
  })

  it('titles each section with the name the owner chose, and shows its firmware', async () => {
    const s = makeTwoDeviceState()
    s.devices[0].label = 'Living room antenna'
    s.devices[0].firmware = updateAvailable
    const user = userEvent.setup()
    renderWithApp(<TunerPage />, s)
    const antenna = screen.getByRole('region', { name: 'Living room antenna' })
    await user.click(within(antenna).getByRole('button', { name: /^Living room antenna/ }))
    await user.click(within(screen.getByRole('region', { name: 'Cable HDHR' })).getByRole('button', { name: /^Cable HDHR/ }))
    expect(screen.getByRole('region', { name: 'Firmware — Living room antenna' })).toBeInTheDocument()
    expect(screen.getByText('Update available · 26 Mar 2026')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Firmware — Cable HDHR' })).toBeInTheDocument()
  })

  it('shows the firmware section for a single tuner without naming it', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.getByRole('region', { name: 'Firmware' })).toBeInTheDocument()
  })

  it('is titled Tuners, like the navigation, even with a single named tuner', () => {
    const s = makeState()
    s.devices[0].label = 'Living room antenna'
    renderWithApp(<TunerPage />, s)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
    expect(screen.getByRole('region', { name: 'Manage' })).toBeInTheDocument()
  })
})

describe('TunerPage channel logos', () => {
  it('shows the logo beside each watched channel that has one', () => {
    const s = makeState()
    s.devices[0].guide = { '5.1': { name: 'WFRV', icon: true }, '2.1': { name: 'WBAY', icon: false } }
    renderWithApp(<TunerPage />, s)
    const logos = screen.getAllByRole('img', { name: /logo/ })
    expect(logos).toHaveLength(1) // 5.1 only: 2.1 has no logo, the idle tuners have no channel
    expect(logos[0]).toHaveAttribute('src', channelIconUrl('antenna', '5.1'))
    expect(logos[0].closest('li')).toHaveTextContent('5.1')
  })

  it('shows no logos when the guide is off or empty', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.queryByRole('img', { name: /logo/ })).not.toBeInTheDocument()
  })
})

describe('TunerPage', () => {
  describe('naming a tuner', () => {
    it('saves the name, trimmed, on that tuner', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<TunerPage />, makeState())

      const box = screen.getByLabelText('Name')
      expect(saveButton(section('Manage'))).toBeNull()
      await user.type(box, '  Antenna (Steve)  ')
      await user.click(saveButton(section('Manage'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/antenna'), body: { label: 'Antenna (Steve)' } })
      expect(await screen.findByText('Settings saved')).toBeInTheDocument()
    })

    it('saves with Enter', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<TunerPage />, makeState())
      await user.type(screen.getByLabelText('Name'), 'Antenna{Enter}')
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: 'Antenna' }))
    })

    it('can be cleared to go back to the tuner’s own name', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      const s = makeState()
      s.devices[0].label = 'Antenna (Steve)'
      renderWithApp(<TunerPage />, s)

      expect(screen.getByLabelText('Name')).toHaveValue('Antenna (Steve)')
      await user.clear(screen.getByLabelText('Name'))
      await user.click(saveButton(section('Manage'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: '' }))
    })

    it('stops at 60 characters, the most the server accepts', () => {
      renderWithApp(<TunerPage />, makeState())
      expect(screen.getByLabelText('Name')).toHaveAttribute('maxlength', '60')
    })

    it("shows the server's objection on the row", async () => {
      const { fn } = stubFetch({ 'PATCH /devices/antenna': () => ({ status: 400, body: { error: 'name for "antenna": can\'t contain control characters' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<TunerPage />, makeState())
      await user.type(screen.getByLabelText('Name'), 'x')
      await user.click(saveButton(section('Manage'))!)
      expect(await within(section('Manage')).findByRole('alert')).toHaveTextContent('control characters')
    })
  })
})

describe('the port', () => {
  it('shows the port number, and Remove is red', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.getByLabelText('Port')).toHaveValue(5004)
    expect(screen.getByRole('button', { name: /^Remove / }).className).toContain('text-destructive')
  })

  it('saves a new port on that tuner, then moves the page there', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<TunerPage />, makeState())
    const box = screen.getByLabelText('Port')
    await user.clear(box)
    await user.type(box, '5010')
    await user.click(saveButton(section('Manage'))!)
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
    renderWithApp(<TunerPage />, s)
    await user.clear(screen.getByLabelText('Port'))
    await user.type(screen.getByLabelText('Port'), '5010')
    await user.click(saveButton(section('Manage'))!)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ listen: '127.0.0.1:5010' }))
  })

  it('rejects nonsense locally, and stays put when the server refuses', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ status: 409, body: { error: "can't listen on :80: in use" } }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<TunerPage />, makeState())
    const box = screen.getByLabelText('Port')
    await user.clear(box)
    await user.type(box, '70000')
    await user.click(saveButton(section('Manage'))!)
    expect(within(section('Manage')).getByText('Enter a port from 1 to 65535.')).toBeInTheDocument()
    expect(calls).toHaveLength(0)

    await user.clear(box)
    await user.type(box, '80')
    await user.click(saveButton(section('Manage'))!)
    expect(await within(section('Manage')).findByRole('alert')).toHaveTextContent("can't listen on :80: in use")
    expect(moveToPort).not.toHaveBeenCalled()
  })
})
