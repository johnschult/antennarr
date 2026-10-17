import { screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { makeState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { LivePage } from './live'

describe('LivePage', () => {
  it('summarises what is on air', () => {
    renderWithApp(<LivePage />, makeState())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('2 of 4 tuners live')
    expect(screen.getByText('1 remote · 1 local · 1 held for you · 27 Mbps')).toBeInTheDocument()
    expect(screen.getByText('upload to viewers')).toBeInTheDocument()
  })

  it('lays the tuners out as tiles with the right holders', () => {
    renderWithApp(<LivePage />, makeState())
    const tiles = within(screen.getByRole('region', { name: 'Tuners' })).getAllByTestId('tuner-tile')
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
    s.tuner.events = []
    renderWithApp(<LivePage />, s)
    expect(screen.getByText(/No viewers yet/)).toBeInTheDocument()
    expect(screen.getByText('Nothing has happened yet.')).toBeInTheDocument()
  })

  it('dims the tiles when the tuner is offline', () => {
    const s = makeState()
    s.tuner.online = false
    renderWithApp(<LivePage />, s)
    for (const t of screen.getAllByTestId('tuner-tile')) expect(t).toHaveClass('opacity-50')
  })

  it('handles a state with no tuners reported at all', () => {
    const s = makeState()
    s.tuner = { ...s.tuner, tunerCount: 0, slots: [], online: false }
    s.summary = { total: 0, inUse: 0, remote: 0, local: 0, reserved: 1, free: 0 }
    renderWithApp(<LivePage />, s)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('0 of 0 tuners live')
    expect(screen.queryAllByTestId('tuner-tile')).toHaveLength(0)
  })
})
