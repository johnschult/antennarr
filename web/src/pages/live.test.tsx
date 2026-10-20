import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { makeState, makeTwoDeviceState, updateAvailable } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { channelIconUrl, programPosterUrl } from '@/lib/api'
import { LivePage } from './live'


function named() {
  const s = makeTwoDeviceState()
  s.devices[0].label = 'Living room antenna'
  s.devices[1].label = 'Cable box'
  return s
}

describe('LivePage', () => {
  it('summarizes what is on air', () => {
    renderWithApp(<LivePage />, makeState())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('2 of 4 tuners live')
    expect(screen.getByText('1 remote · 1 local · 1 held for you · 27 Mbps')).toBeInTheDocument()
    expect(screen.queryByText('upload to viewers')).not.toBeInTheDocument() // no overall chart; each live tile has its own
  })

  it('lays the tuners out as tiles with the right holders', () => {
    renderWithApp(<LivePage />, makeState())
    const tiles = within(screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })).getAllByTestId('tuner-tile')
    expect(tiles.map((t) => t.getAttribute('data-owner'))).toEqual(['local', 'viewer', 'reserved', 'free'])
    expect(within(tiles[1]).getAllByText('John').length).toBeGreaterThan(0)
  })

  it('lists viewers with how many tuners they are using and when they were last seen', () => {
    renderWithApp(<LivePage />, makeState())
    const john = screen.getByText('John', { selector: 'div.font-medium' }).closest('div.flex.items-center.gap-4')!
    expect(within(john as HTMLElement).getByText('Watching')).toBeInTheDocument()
    expect(within(john as HTMLElement).getByRole('img', { name: '1 of 2 tuners in use' })).toBeInTheDocument()
    expect(screen.getByText(/waiting for first contact/)).toBeInTheDocument() // Mom
  })

  it('shows the timeline newest first', () => {
    renderWithApp(<LivePage />, makeState())
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent('Mom was refused a tuner')
    expect(items[1]).toHaveTextContent('Tuner online')
  })

  it('says so when there is nothing to show yet', () => {
    const s = makeState({ viewers: [] })
    s.devices[0].tuner.events = []
    renderWithApp(<LivePage />, s)
    expect(screen.getByText(/No viewers yet/)).toBeInTheDocument()
    expect(screen.getByText('Nothing has happened yet.')).toBeInTheDocument()
  })

  it('dims the tiles when the tuner is offline', () => {
    const s = makeState()
    s.devices[0].tuner.online = false
    renderWithApp(<LivePage />, s)
    for (const t of screen.getAllByTestId('tuner-tile')) expect(t).toHaveClass('opacity-50')
  })

  it('handles a state with no tuners reported at all', () => {
    const s = makeState()
    s.devices[0].tuner = { ...s.devices[0].tuner, tunerCount: 0, slots: [], online: false }
    s.devices[0].summary = { total: 0, inUse: 0, remote: 0, local: 0, reserved: 1, free: 0 }
    renderWithApp(<LivePage />, s)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('0 of 0 tuners live')
    expect(screen.queryAllByTestId('tuner-tile')).toHaveLength(0)
  })

  describe('with several devices', () => {
    it('adds up the headline numbers and gives each device its own section', () => {
      renderWithApp(<LivePage />, makeTwoDeviceState())
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('2 of 6 tuners live')
      expect(screen.getByText('1 remote · 1 local · 1 held for you · 27 Mbps')).toBeInTheDocument()

      const antenna = screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })
      const cable = screen.getByRole('region', { name: 'Tuners on Cable HDHR' })
      expect(within(antenna).getAllByTestId('tuner-tile')).toHaveLength(4)
      expect(within(cable).getAllByTestId('tuner-tile')).toHaveLength(2)
      expect(within(antenna).getByRole('heading', { name: 'Steve FLEX 4K' })).toBeInTheDocument()
      // The model is the title, so it is said once, not again beside it.
      expect(within(antenna).getAllByText('Steve FLEX 4K')).toHaveLength(1)
      expect(antenna).toHaveTextContent('2 of 4 live')
      expect(cable).toHaveTextContent('0 of 2 live')
    })

    it('dims only the tiles of the device that is offline', () => {
      const s = makeTwoDeviceState()
      s.devices[1].tuner.online = false
      renderWithApp(<LivePage />, s)
      const dimmed = (name: string) => within(screen.getByRole('region', { name })).getAllByTestId('tuner-tile').map((t) => t.classList.contains('opacity-50'))
      expect(dimmed('Tuners on Steve FLEX 4K').every((d) => !d)).toBe(true)
      expect(dimmed('Tuners on Cable HDHR').every((d) => d)).toBe(true)
    })

    it("shows each viewer's use and limit on every device, using per-device overrides", () => {
      renderWithApp(<LivePage />, makeTwoDeviceState())
      const row = screen.getByText('John', { selector: 'div.font-medium' }).closest('div.flex.items-center.gap-4') as HTMLElement
      // John: default limit 2, but 2 on cable too via the override; he is using 1 on the antenna box.
      expect(within(row).getByRole('img', { name: 'Steve FLEX 4K: 1 of 2 tuners in use' })).toBeInTheDocument()
      expect(within(row).getByRole('img', { name: 'Cable HDHR: 0 of 2 tuners in use' })).toBeInTheDocument()
      // Mom: limit 1 everywhere.
      const mom = screen.getByText('Mom', { selector: 'div.font-medium' }).closest('div.flex.items-center.gap-4') as HTMLElement
      expect(within(mom).getByRole('img', { name: 'Cable HDHR: 0 of 1 tuners in use' })).toBeInTheDocument()
    })

    it('merges the timelines, newest first, saying which device each event is from', () => {
      renderWithApp(<LivePage />, makeTwoDeviceState())
      const items = screen.getAllByRole('listitem')
      expect(items).toHaveLength(3)
      expect(items[0]).toHaveTextContent('Mom was refused a tuner') // 09:30, antenna
      expect(items[0]).toHaveTextContent('Steve FLEX 4K')
      expect(items[1]).toHaveTextContent('Tuner online at http://192.168.1.133') // 08:05, cable
      expect(items[1]).toHaveTextContent('Cable HDHR')
      expect(items[2]).toHaveTextContent('Tuner online at http://192.168.1.132') // 08:00
    })

    it('does not say which device an event is from when there is only one', () => {
      renderWithApp(<LivePage />, makeState())
      for (const item of screen.getAllByRole('listitem')) expect(item).not.toHaveTextContent('Steve FLEX 4K')
    })
  })

  describe('with names the owner chose', () => {
    it('uses them for the sections, the viewer rows and the timeline', () => {
      renderWithApp(<LivePage />, named())
      expect(screen.getByRole('region', { name: 'Tuners on Living room antenna' })).toBeInTheDocument()
      expect(screen.getByRole('region', { name: 'Tuners on Cable box' })).toBeInTheDocument()
      expect(screen.queryByRole('region', { name: 'Tuners on Steve FLEX 4K' })).not.toBeInTheDocument()

      const row = screen.getByText('John', { selector: 'div.font-medium' }).closest('div.flex.items-center.gap-4') as HTMLElement
      expect(within(row).getByRole('img', { name: 'Living room antenna: 1 of 2 tuners in use' })).toBeInTheDocument()
      expect(within(row).getByRole('img', { name: 'Cable box: 0 of 2 tuners in use' })).toBeInTheDocument()

      const items = screen.getAllByRole('listitem')
      expect(items[0]).toHaveTextContent('Living room antenna')
      expect(items[1]).toHaveTextContent('Cable box')
    })
  })

  describe('firmware', () => {
    it("flags a tuner that has an update, and only that one", () => {
      const s = makeTwoDeviceState()
      s.devices[0].firmware = updateAvailable
      renderWithApp(<LivePage />, s)
      expect(within(screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })).getByRole('link', { name: /Firmware update/ })).toHaveAttribute('href', '/tuners/antenna/firmware')
      expect(within(screen.getByRole('region', { name: 'Tuners on Cable HDHR' })).queryByRole('link', { name: /Firmware/ })).not.toBeInTheDocument()
    })

    it('links each tuner’s name to its own page', () => {
      renderWithApp(<LivePage />, makeTwoDeviceState())
      expect(within(screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })).getByRole('link', { name: 'Steve FLEX 4K' })).toHaveAttribute('href', '/tuners/antenna')
      expect(within(screen.getByRole('region', { name: 'Tuners on Cable HDHR' })).getByRole('link', { name: 'Cable HDHR' })).toHaveAttribute('href', '/tuners/cable')
    })
  })

  it("always gives a tuner a header: the model when it has no name of its own, so it is said once", () => {
    renderWithApp(<LivePage />, makeState())
    const section = screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })
    expect(within(section).getByRole('heading', { level: 2, name: 'Steve FLEX 4K' })).toBeInTheDocument()
    expect(within(section).getAllByText('Steve FLEX 4K')).toHaveLength(1)
    expect(section).toHaveTextContent('2 of 4 live')
    expect(section).toHaveTextContent('192.168.1.132') // where it is: the top bar no longer says
  })

  it('shows the name you gave a tuner as its title, with the model beside it', () => {
    const s = makeState()
    s.devices[0].label = 'Living room antenna'
    renderWithApp(<LivePage />, s)
    const section = screen.getByRole('region', { name: 'Tuners on Living room antenna' })
    expect(within(section).getByRole('heading', { level: 2, name: 'Living room antenna' })).toBeInTheDocument()
    expect(section).toHaveTextContent('Steve FLEX 4K · 2 of 4 live')
  })

  it('does not say the model twice when the name you gave is the model', () => {
    const s = makeState()
    s.devices[0].label = 'Steve FLEX 4K'
    renderWithApp(<LivePage />, s)
    expect(within(screen.getByRole('region', { name: 'Tuners on Steve FLEX 4K' })).getAllByText('Steve FLEX 4K')).toHaveLength(1)
  })

})

describe('LivePage summary', () => {
  it('says what is on the air now, and how many tuners are live', () => {
    renderWithApp(<LivePage />, makeState())
    expect(screen.getByText('On the air now')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/\d+ of \d+ tuners live/)
  })
})

describe('LivePage headings', () => {
  it('has an icon on the Viewers and Timeline headings, which the page names without', () => {
    renderWithApp(<LivePage />, makeState())
    for (const name of ['Viewers', 'Timeline']) {
      const h = screen.getByRole('heading', { name })
      expect(h.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
    }
  })
})

describe('LivePage guide', () => {
  it('passes each watched channel its logo and program', () => {
    const s = makeState()
    s.devices[0].guide = {
      '5.1': { name: 'WFRV', icon: true, now: { title: 'Evening News', start: '2026-10-17T17:00:00Z', stop: '2026-10-17T18:00:00Z' } },
    }
    renderWithApp(<LivePage />, s)
    expect(screen.getByRole('img', { name: /logo/ })).toHaveAttribute('src', channelIconUrl('antenna', '5.1'))
    expect(screen.getByTestId('tile-now')).toHaveTextContent('Evening News')
  })

  it("passes a program's picture on, through Yagi, only when the guide has one", () => {
    const s = makeState()
    s.devices[0].guide = {
      '5.1': { name: 'WFRV', icon: true, now: { title: 'Evening News', start: '2026-10-17T17:00:00Z', stop: '2026-10-17T18:00:00Z', poster: 'ABC123' } },
    }
    const { container, unmount } = renderWithApp(<LivePage />, s)
    expect(container.querySelector(`img[src="${programPosterUrl('antenna', '5.1', 'ABC123')}"]`)).not.toBeNull()
    unmount()

    s.devices[0].guide['5.1'].now!.poster = undefined
    const again = renderWithApp(<LivePage />, s)
    expect(again.container.querySelector('img[src*="/poster/"]')).toBeNull()
  })
})
