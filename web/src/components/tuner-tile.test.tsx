import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Tile } from '@/lib/slots'
import { idleSlot, localSlot, remoteSlot } from '@/test/fixtures'
import { TunerTile } from './tuner-tile'

const tile = (over: Partial<Tile>): Tile => ({ index: 0, slot: idleSlot(0), owner: { kind: 'free' }, ...over })

describe('TunerTile', () => {
  it("shows a viewer's channel, who has it and the rate", () => {
    render(<TunerTile tile={tile({ index: 1, slot: remoteSlot(1), owner: { kind: 'viewer', viewer: 'John' } })} />)
    expect(screen.getByText('TUNER 2')).toBeInTheDocument()
    expect(screen.getByText('5.1')).toBeInTheDocument()
    expect(screen.getByText('WFRV')).toBeInTheDocument()
    expect(screen.getAllByText('John')).toHaveLength(2) // the badge and the footer
    expect(screen.getByText('14 Mbps')).toBeInTheDocument()
    expect(screen.getByTestId('tuner-tile')).toHaveAttribute('data-owner', 'viewer')
  })

  it("shows the owner's own device by its address", () => {
    render(<TunerTile tile={tile({ slot: localSlot(0), owner: { kind: 'local' } })} />)
    expect(screen.getByText('Local')).toBeInTheDocument()
    expect(screen.getByText('192.168.1.77')).toBeInTheDocument()
    expect(screen.getByText('WBAY')).toBeInTheDocument()
  })

  it('says a tuner with no channel yet is tuning', () => {
    render(<TunerTile tile={tile({ slot: { ...remoteSlot(0), channel: undefined, channelName: undefined }, owner: { kind: 'remote' } })} />)
    expect(screen.getByText('Tuning…')).toBeInTheDocument()
  })

  it('shows reserved and free tuners without a channel', () => {
    const { rerender } = render(<TunerTile tile={tile({ owner: { kind: 'reserved' } })} />)
    expect(screen.getByText('Held for your TVs')).toBeInTheDocument()
    expect(screen.getByText('Reserved')).toBeInTheDocument()

    rerender(<TunerTile tile={tile({ owner: { kind: 'free' } })} />)
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.queryByText('Mbps')).not.toBeInTheDocument()
  })

  it('copes with a tuner that has not reported a slot yet', () => {
    render(<TunerTile tile={tile({ slot: null })} />)
    expect(screen.getByText('Ready')).toBeInTheDocument()
  })

  it('is dimmed when the tuner is offline', () => {
    const { rerender } = render(<TunerTile tile={tile({})} dimmed />)
    expect(screen.getByTestId('tuner-tile')).toHaveClass('opacity-50')
    rerender(<TunerTile tile={tile({})} />)
    expect(screen.getByTestId('tuner-tile')).not.toHaveClass('opacity-50')
  })
})
