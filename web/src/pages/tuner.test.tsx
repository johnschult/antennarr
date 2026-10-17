import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { idleSlot, localSlot, makeState, remoteSlot } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
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
    s.tuner.slots = [
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
    s.tuner = { ...s.tuner, online: false, id: '', friendlyName: undefined, model: undefined, slots: [] }
    renderWithApp(<TunerPage />, s)
    expect(screen.getByText('Not yet seen')).toBeInTheDocument()
    expect(screen.getByText('Offline')).toBeInTheDocument()
    expect(screen.getByText('No tuner data yet.')).toBeInTheDocument()
  })
})
