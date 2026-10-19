import { fireEvent, render, screen, within } from '@testing-library/react'
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
    expect(screen.getAllByText('John')).toHaveLength(1) // the name is in the pill only; the footer says where it is going
    expect(screen.getByText('14 Mbps')).toBeInTheDocument()
    expect(screen.getByTestId('tuner-tile')).toHaveAttribute('data-owner', 'viewer')
  })

  it("shows where a viewer's stream is going, without the port, and nothing when it can't tell", () => {
    const stream = { id: 1, device: 'antenna', viewer: 'John', client: '100.67.117.32:4000', path: '/auto/v5.1', channel: '5.1', started: '2026-10-17T10:00:00Z' }
    const { rerender } = render(<TunerTile tile={tile({ slot: remoteSlot(1), owner: { kind: 'viewer', viewer: 'John' }, stream })} />)
    expect(screen.getByText('100.67.117.32')).toBeInTheDocument()
    expect(screen.queryByText(/:4000/)).not.toBeInTheDocument()

    rerender(<TunerTile tile={tile({ slot: remoteSlot(1), owner: { kind: 'remote' } })} />)
    expect(screen.getAllByText('Remote')).toHaveLength(1) // the pill; the footer doesn't repeat it
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
    expect(screen.getByText('Idle')).toBeInTheDocument()
    expect(screen.queryByText('Mbps')).not.toBeInTheDocument()
  })

  it('copes with a tuner that has not reported a slot yet', () => {
    render(<TunerTile tile={tile({ slot: null })} />)
    expect(screen.getByText('Idle')).toBeInTheDocument()
  })

  it('is dimmed when the tuner is offline', () => {
    const { rerender } = render(<TunerTile tile={tile({})} dimmed />)
    expect(screen.getByTestId('tuner-tile')).toHaveClass('opacity-50')
    rerender(<TunerTile tile={tile({})} />)
    expect(screen.getByTestId('tuner-tile')).not.toHaveClass('opacity-50')
  })

  describe('signal quality trend', () => {
    const live = (quality: number | undefined, channel = '5.1') =>
      tile({ slot: { ...remoteSlot(0), channel, signalStrength: 80, signalQuality: quality }, owner: { kind: 'remote' } })
    const line = () => within(screen.getByTestId('tile-signal-trend')).getByRole('img', { name: /Signal quality/ })

    it('starts empty, then draws a line as samples arrive, in the second chart color', () => {
      const { rerender } = render(<TunerTile tile={live(90)} tick={1} />)
      expect(line()).toHaveAccessibleName('Signal quality: no data yet')
      rerender(<TunerTile tile={live(70)} tick={2} />)
      expect(line()).toHaveAccessibleName('Signal quality')
      expect(line().querySelectorAll('path')).toHaveLength(2)
      expect(line()).toHaveClass('text-chart-2')
    })

    it('starts a new line when the channel changes', () => {
      const { rerender } = render(<TunerTile tile={live(90)} tick={1} />)
      rerender(<TunerTile tile={live(90)} tick={2} />)
      expect(line()).toHaveAccessibleName('Signal quality')
      rerender(<TunerTile tile={live(90, '7.1')} tick={3} />)
      expect(line()).toHaveAccessibleName('Signal quality: no data yet')
    })

    it('is left out when the tuner reports no quality', () => {
      render(<TunerTile tile={live(undefined)} tick={1} />)
      expect(screen.queryByTestId('tile-signal-trend')).not.toBeInTheDocument()
      expect(screen.getByTestId('tile-signal')).toBeInTheDocument()
    })

    it('leaves the bitrate line in the first chart color', () => {
      const { rerender } = render(<TunerTile tile={live(90)} tick={1} />)
      rerender(<TunerTile tile={live(90)} tick={2} />)
      expect(within(screen.getByTestId('tile-bitrate')).getByRole('img')).toHaveClass('text-chart-1')
    })
  })

  describe('bitrate line', () => {
    const live = (bps: number, channel = '5.1') => tile({ slot: { ...remoteSlot(0), channel, bitsPerSecond: bps }, owner: { kind: 'remote' } })
    const line = () => within(screen.getByTestId('tile-bitrate')).getByRole('img', { name: /Bitrate/ })

    it('starts empty, then draws a line as samples arrive', () => {
      const { rerender } = render(<TunerTile tile={live(8_000_000)} tick={1} />)
      expect(line()).toHaveAccessibleName('Bitrate: no data yet')
      rerender(<TunerTile tile={live(12_000_000)} tick={2} />)
      expect(line()).toHaveAccessibleName('Bitrate')
      expect(line().querySelectorAll('path')).toHaveLength(2)
    })

    it('shows the rate on the line itself, once, and not in the footer', () => {
      render(<TunerTile tile={live(12_000_000)} tick={1} />)
      expect(within(screen.getByTestId('tile-bitrate')).getByText('12 Mbps')).toBeInTheDocument()
      expect(screen.getAllByText('12 Mbps')).toHaveLength(1)
    })

    it('does not add a sample until the state has updated', () => {
      const { rerender } = render(<TunerTile tile={live(8_000_000)} tick={1} />)
      rerender(<TunerTile tile={live(9_000_000)} tick={1} />)
      expect(line()).toHaveAccessibleName('Bitrate: no data yet')
    })

    it('starts a new line when the channel changes', () => {
      const { rerender } = render(<TunerTile tile={live(8_000_000)} tick={1} />)
      rerender(<TunerTile tile={live(8_000_000)} tick={2} />)
      expect(line()).toHaveAccessibleName('Bitrate')
      rerender(<TunerTile tile={live(8_000_000, '7.1')} tick={3} />)
      expect(line()).toHaveAccessibleName('Bitrate: no data yet')
    })

    it('starts a new line when the stream ends and another begins', () => {
      const { rerender } = render(<TunerTile tile={live(8_000_000)} tick={1} />)
      rerender(<TunerTile tile={live(8_000_000)} tick={2} />)
      rerender(<TunerTile tile={tile({ owner: { kind: 'free' } })} tick={3} />)
      expect(screen.queryByTestId('tile-bitrate')).not.toBeInTheDocument()
      rerender(<TunerTile tile={live(8_000_000)} tick={4} />)
      expect(line()).toHaveAccessibleName('Bitrate: no data yet')
    })

    it('stays empty, without breaking, when the tuner reports no rate', () => {
      render(<TunerTile tile={tile({ slot: { ...remoteSlot(0), bitsPerSecond: undefined }, owner: { kind: 'remote' } })} tick={1} />)
      expect(line()).toHaveAccessibleName('Bitrate: no data yet')
    })

    it('is not shown on idle tuners', () => {
      render(<TunerTile tile={tile({ owner: { kind: 'free' } })} tick={1} />)
      expect(screen.queryByTestId('tile-bitrate')).not.toBeInTheDocument()
    })
  })

  describe('guide', () => {
    const watching = tile({ slot: remoteSlot(1), owner: { kind: 'remote' } })
    const entry = { name: 'WFRV', icon: true, now: { title: "Rosie's Rules", subTitle: 'The Catrina Mystery', start: '2026-10-17T17:30:00Z', stop: '2026-10-17T18:00:00Z' } }
    const icon = '/yagi/api/devices/antenna/guide/5.1/icon'

    it('shows the channel logo and what is on', () => {
      render(<TunerTile tile={watching} guide={entry} iconSrc={icon} />)
      expect(screen.getByRole('img', { name: 'WFRV logo' })).toHaveAttribute('src', icon)
      const now = screen.getByTestId('tile-now')
      expect(now).toHaveTextContent("Rosie's Rules")
      expect(now).toHaveAttribute('title', "Rosie's Rules: The Catrina Mystery")
    })

    it('shows neither when the guide knows nothing about the channel', () => {
      render(<TunerTile tile={watching} />)
      expect(screen.queryByRole('img', { name: /logo/ })).not.toBeInTheDocument()
      expect(screen.queryByTestId('tile-now')).not.toBeInTheDocument()
    })

    it('shows the program without a logo when there is none, or it will not load', () => {
      const { rerender } = render(<TunerTile tile={watching} guide={{ ...entry, icon: false }} iconSrc={icon} />)
      expect(screen.queryByRole('img', { name: /logo/ })).not.toBeInTheDocument()
      expect(screen.getByTestId('tile-now')).toBeInTheDocument()

      rerender(<TunerTile tile={watching} guide={entry} iconSrc={icon} />)
      fireEvent.error(screen.getByRole('img', { name: 'WFRV logo' }))
      expect(screen.queryByRole('img', { name: /logo/ })).not.toBeInTheDocument()
      expect(screen.getByTestId('tile-now')).toBeInTheDocument()
    })

    it('shows the logo without a program in a gap in the guide', () => {
      render(<TunerTile tile={watching} guide={{ ...entry, now: undefined }} iconSrc={icon} />)
      expect(screen.getByRole('img', { name: 'WFRV logo' })).toBeInTheDocument()
      expect(screen.queryByTestId('tile-now')).not.toBeInTheDocument()
    })

    it('tries the logo again for a different channel after one failed', () => {
      const { rerender } = render(<TunerTile tile={watching} guide={entry} iconSrc={icon} />)
      fireEvent.error(screen.getByRole('img', { name: 'WFRV logo' }))
      rerender(<TunerTile tile={watching} guide={entry} iconSrc={icon.replace('5.1', '7.1')} />)
      expect(screen.getByRole('img', { name: 'WFRV logo' })).toBeInTheDocument()
    })

    it('shows no guide on idle tuners', () => {
      render(<TunerTile tile={tile({ owner: { kind: 'free' } })} guide={entry} iconSrc={icon} />)
      expect(screen.queryByTestId('tile-now')).not.toBeInTheDocument()
    })
  })

  describe('program details', () => {
    const watching = tile({ slot: remoteSlot(1), owner: { kind: 'remote' } })
    const start = '2026-10-17T17:30:00Z'
    const stop = '2026-10-17T18:30:00Z'
    const full = {
      name: 'WFRV',
      icon: true,
      now: { title: "Rosie's Rules", subTitle: 'The Catrina Mystery', episode: 'S01E32', category: 'Series', description: 'For the Day of the Dead celebration.', start, stop, poster: 'C22744932ENOGRU' },
    }
    const poster = '/yagi/api/devices/antenna/guide/5.1/poster/C22744932ENOGRU'
    const at = (iso: string) => new Date(iso)

    it('shows the category, the episode with its title, and the description', () => {
      render(<TunerTile tile={watching} guide={full} now={at('2026-10-17T17:45:00Z')} />)
      expect(screen.getByText('Series')).toBeInTheDocument()
      expect(screen.getByText('S01E32 · The Catrina Mystery')).toBeInTheDocument()
      expect(screen.getByText('For the Day of the Dead celebration.')).toBeInTheDocument()
    })

    it('leaves out what the guide does not say', () => {
      const bare = { ...full, now: { title: 'Evening News', start, stop } }
      render(<TunerTile tile={watching} guide={bare} now={at('2026-10-17T17:45:00Z')} />)
      expect(screen.getByTestId('tile-now')).toHaveTextContent('Evening News')
      expect(screen.queryByText('Series')).not.toBeInTheDocument()
      expect(screen.queryByText(/·/)).not.toBeInTheDocument()
    })

    it('shows how far into the program it is, and how long is left', () => {
      render(<TunerTile tile={watching} guide={full} now={at('2026-10-17T17:45:00Z')} />)
      const bar = screen.getByRole('progressbar', { name: 'How far into the program' })
      expect(bar).toHaveAttribute('aria-valuenow', '25')
      expect(screen.getByTestId('tile-progress')).toHaveTextContent('45 min left')
    })

    it('keeps the bar within the program when the clock is outside it', () => {
      const { rerender } = render(<TunerTile tile={watching} guide={full} now={at('2026-10-17T16:00:00Z')} />)
      expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')
      rerender(<TunerTile tile={watching} guide={full} now={at('2026-10-17T19:00:00Z')} />)
      expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
      expect(screen.getByTestId('tile-progress')).toHaveTextContent('ending')
    })

    it('has no bar when the time it is measured against is unknown', () => {
      render(<TunerTile tile={watching} guide={full} now={new Date('not a time')} />)
      expect(screen.queryByTestId('tile-progress')).not.toBeInTheDocument()
    })

    it('has no bar for times that make no sense', () => {
      render(<TunerTile tile={watching} guide={{ ...full, now: { ...full.now, start: stop, stop: start } }} now={at('2026-10-17T17:45:00Z')} />)
      expect(screen.queryByTestId('tile-progress')).not.toBeInTheDocument()
    })

    it("draws the program's picture behind the top of the tile, hidden from screen readers", () => {
      render(<TunerTile tile={watching} guide={full} posterSrc={poster} />)
      const img = document.querySelector(`img[src="${poster}"]`)
      expect(img).not.toBeNull()
      expect(img).toHaveAttribute('aria-hidden', 'true')
      expect(img).toHaveAttribute('alt', '')
    })

    it('shows the tile without the picture when there is none, or it will not load', () => {
      const { rerender } = render(<TunerTile tile={watching} guide={full} />)
      expect(document.querySelector('img[src*="/poster/"]')).toBeNull()
      expect(screen.getByTestId('tile-now')).toBeInTheDocument()

      rerender(<TunerTile tile={watching} guide={full} posterSrc={poster} />)
      fireEvent.error(document.querySelector(`img[src="${poster}"]`)!)
      expect(document.querySelector('img[src*="/poster/"]')).toBeNull()
      expect(screen.getByTestId('tile-now')).toHaveTextContent("Rosie's Rules")
    })

    it('tries again for another program after one picture failed', () => {
      const { rerender } = render(<TunerTile tile={watching} guide={full} posterSrc={poster} />)
      fireEvent.error(document.querySelector(`img[src="${poster}"]`)!)
      rerender(<TunerTile tile={watching} guide={full} posterSrc={poster.replace('C22744932ENOGRU', 'NEXT')} />)
      expect(document.querySelector('img[src$="/poster/NEXT"]')).not.toBeNull()
    })

    it('shows no picture on idle tuners', () => {
      render(<TunerTile tile={tile({ owner: { kind: 'free' } })} guide={full} posterSrc={poster} />)
      expect(document.querySelector('img[src*="/poster/"]')).toBeNull()
    })
  })

  it('marks a reserved tuner with a crown in its pill, and a TV on the tile', () => {
    render(<TunerTile tile={tile({ owner: { kind: 'reserved' } })} />)
    expect(document.querySelectorAll('svg.lucide-crown')).toHaveLength(1)
    expect(within(screen.getByTestId('tuner-tile')).getByText('Held for your TVs').parentElement!.querySelector('svg.lucide-tv')).not.toBeNull()
  })

  it('has no icon on a free tuner, and no antennas on a tile', () => {
    render(<TunerTile tile={tile({ owner: { kind: 'free' } })} />)
    expect(within(screen.getByTestId('tuner-tile')).getByText('Idle').parentElement!.querySelector('svg')).toBeNull()
    expect(document.querySelector('svg.lucide-antenna')).toBeNull()
  })

  describe('signal', () => {
    const withSignal = (strength?: number, quality?: number) =>
      tile({ slot: { ...remoteSlot(0), signalStrength: strength, signalQuality: quality }, owner: { kind: 'remote' } })

    it('shows strength in the footer and quality on its trend line, as percentages', () => {
      render(<TunerTile tile={withSignal(92, 88)} />)
      expect(screen.getByTestId('tile-signal')).toHaveTextContent('Signal strength 92%')
      expect(within(screen.getByTestId('tile-signal-trend')).getByText('88%')).toBeInTheDocument()
    })

    it('says nothing special about good reception, and colors trouble', () => {
      render(<TunerTile tile={withSignal(95, 60)} />)
      expect(screen.getByTestId('tile-signal')).toHaveAttribute('data-tone', 'good')
      expect(screen.getByTestId('tile-signal')).not.toHaveClass('text-amber-700', 'text-red-700')
      expect(within(screen.getByTestId('tile-signal-trend')).getByText('60%')).toHaveClass('text-amber-700')
    })

    it('says poor reception is poor', () => {
      render(<TunerTile tile={withSignal(20, 10)} />)
      expect(screen.getByTestId('tile-signal')).toHaveAttribute('data-tone', 'poor')
      expect(screen.getByTestId('tile-signal')).toHaveClass('text-red-700')
      expect(within(screen.getByTestId('tile-signal-trend')).getByText('10%')).toHaveClass('text-red-700')
    })

    it.each([
      [100, 'lucide-signal'],
      [80, 'lucide-signal'],
      [79, 'lucide-signal-high'],
      [60, 'lucide-signal-high'],
      [59, 'lucide-signal-medium'],
      [40, 'lucide-signal-medium'],
      [39, 'lucide-signal-low'],
      [20, 'lucide-signal-low'],
      [19, 'lucide-signal-zero'],
      [0, 'lucide-signal-zero'],
    ])('draws a strength of %i with the %s icon', (strength, cls) => {
      render(<TunerTile tile={withSignal(strength, 90)} />)
      const icon = screen.getByTestId('tile-signal').querySelector('svg')!
      expect(icon).toHaveClass(cls)
      expect(icon.getAttribute('class')?.split(' ').filter((c) => c.startsWith('lucide-signal'))).toEqual([cls])
    })

    it('shows only strength when the tuner gives no quality', () => {
      render(<TunerTile tile={withSignal(70, undefined)} />)
      expect(screen.getByTestId('tile-signal')).toHaveTextContent('Signal strength 70%')
      expect(screen.queryByTestId('tile-signal-trend')).not.toBeInTheDocument()
    })

    it('keeps odd readings within 0 to 100', () => {
      render(<TunerTile tile={withSignal(140, -5)} />)
      expect(screen.getByTestId('tile-signal')).toHaveTextContent('Signal strength 100%')
      expect(within(screen.getByTestId('tile-signal-trend')).getByText('0%')).toBeInTheDocument()
    })

    it('is left out when the tuner reports no signal, and on idle tuners', () => {
      const { rerender } = render(<TunerTile tile={withSignal(undefined, undefined)} />)
      expect(screen.queryByTestId('tile-signal')).not.toBeInTheDocument()
      rerender(<TunerTile tile={tile({ owner: { kind: 'free' } })} />)
      expect(screen.queryByTestId('tile-signal')).not.toBeInTheDocument()
    })
  })
})
