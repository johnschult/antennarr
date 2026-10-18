import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { DeviceInfo } from '@/lib/types'
import { makeDevice, makeState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { DevicePanel } from './device-panel'

afterEach(() => vi.unstubAllGlobals())

const info: DeviceInfo = {
  hardwareModel: 'HDFX-4K',
  model: 'hdhomerun_dvr_atsc3',
  version: '20260326',
  drm: 'SD20230420',
  deviceId: '10ABCC87',
  mac: '00:18:DD:00:00:01',
  ip: '192.168.1.50',
  subnet: '255.255.255.0',
  channelMaps: ['us-bcast', 'us-cable'],
  modulations: ['atsc3', '8vsb'],
  readAt: new Date().toISOString(),
}

const path = 'GET /devices/antenna/info'
const show = (device = makeDevice()) => renderWithApp(<DevicePanel device={device} showName={false} />, makeState({ devices: [device] }))
const panel = () => screen.getByRole('region', { name: 'Device' })

describe('Device panel', () => {
  it('shows what the tuner says about itself', async () => {
    vi.stubGlobal('fetch', stubFetch({ [path]: () => ({ body: info }) }).fn)
    show()
    expect(await within(panel()).findByText('HDFX-4K')).toBeInTheDocument()
    for (const v of ['SD20230420', '00:18:DD:00:00:01', '192.168.1.50', '255.255.255.0', 'us-bcast, us-cable', 'atsc3, 8vsb', 'hdhomerun_dvr_atsc3']) {
      expect(within(panel()).getByText(v)).toBeInTheDocument()
    }
    expect(within(panel()).getByText(/Read-only: nothing here changes the tuner/)).toBeInTheDocument()
  })

  it('leaves out what could not be read and says why', async () => {
    const partial = { ...info, drm: undefined, mac: undefined, ip: undefined, subnet: undefined, problems: ["the tuner's System Menu page isn't laid out as expected"] }
    vi.stubGlobal('fetch', stubFetch({ [path]: () => ({ body: partial }) }).fn)
    show()
    await within(panel()).findByText('HDFX-4K')
    expect(within(panel()).queryByText('MAC address')).not.toBeInTheDocument()
    expect(within(panel()).getByText(/Not shown: the tuner's System Menu page isn't laid out as expected\./)).toBeInTheDocument()
  })

  it('says so when the tuner could not be read', async () => {
    vi.stubGlobal('fetch', stubFetch({ [path]: () => ({ status: 502, body: { error: "couldn't read anything from the tuner" } }) }).fn)
    show()
    expect(await within(panel()).findByRole('alert')).toHaveTextContent("couldn't read anything from the tuner")
  })

  it('does not ask an offline tuner', () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    show(makeDevice({ tuner: { ...makeDevice().tuner, online: false } }))
    expect(within(panel()).getByText(/The tuner is offline/)).toBeInTheDocument()
    expect(calls).toHaveLength(0)
  })

  it('refreshes on request, past the short memory', async () => {
    const { fn, calls } = stubFetch({
      [path]: () => ({ body: info }),
      'GET /devices/antenna/info?refresh=1': () => ({ body: { ...info, ip: '192.168.1.99' } }),
    })
    vi.stubGlobal('fetch', fn)
    show()
    await within(panel()).findByText('192.168.1.50')
    await userEvent.setup().click(within(panel()).getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(within(panel()).getByText('192.168.1.99')).toBeInTheDocument())
    expect(calls.filter((c) => c.url.includes('refresh=1'))).toHaveLength(1)
  })

  it('names the tuner when there are several', async () => {
    vi.stubGlobal('fetch', stubFetch({ [path]: () => ({ body: info }) }).fn)
    const device = makeDevice()
    renderWithApp(<DevicePanel device={device} showName />, makeState({ devices: [device] }))
    expect(await screen.findByRole('region', { name: /^Device — / })).toBeInTheDocument()
  })
})
