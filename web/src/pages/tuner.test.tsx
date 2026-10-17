import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { idleSlot, localSlot, makeState, makeTwoDeviceState, remoteSlot, updateAvailable } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { channelIconUrl } from '@/lib/api'
import { TunerPage } from './tuner'

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
    const rows = screen.getAllByRole('row').slice(1)
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

  it('has a section for each device when there are several', () => {
    renderWithApp(<TunerPage />, makeTwoDeviceState())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
    const antenna = screen.getByRole('region', { name: 'antenna' })
    const cable = screen.getByRole('region', { name: 'cable' })
    expect(within(antenna).getByText('10ABCC87')).toBeInTheDocument()
    expect(within(cable).getByText('1111AAA2')).toBeInTheDocument()
    expect(within(cable).getByText('HDHR5-4DT')).toBeInTheDocument()
    expect(within(antenna).getAllByRole('row')).toHaveLength(5) // header + 4 tuners
    expect(within(cable).getAllByRole('row')).toHaveLength(3) // header + 2 tuners
  })

  it('titles each section with the name the owner chose, and shows its firmware', () => {
    const s = makeTwoDeviceState()
    s.devices[0].label = 'Living room antenna'
    s.devices[0].firmware = updateAvailable
    renderWithApp(<TunerPage />, s)
    const antenna = screen.getByRole('region', { name: 'Living room antenna' })
    expect(within(antenna).getByRole('heading', { name: 'Living room antenna' })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Firmware — Living room antenna' })).toBeInTheDocument()
    expect(screen.getByText('Update available · 26 Mar 2026')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Firmware — cable' })).toBeInTheDocument()
  })

  it('shows the firmware section for a single tuner without naming it', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.getByRole('region', { name: 'Firmware' })).toBeInTheDocument()
  })

  it('titles the page with the name given to a single tuner', () => {
    const s = makeState()
    s.devices[0].label = 'Living room antenna'
    renderWithApp(<TunerPage />, s)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Living room antenna')
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
    expect(logos[0].closest('tr')).toHaveTextContent('5.1')
  })

  it('shows no logos when the guide is off or empty', () => {
    renderWithApp(<TunerPage />, makeState())
    expect(screen.queryByRole('img', { name: /logo/ })).not.toBeInTheDocument()
  })
})
