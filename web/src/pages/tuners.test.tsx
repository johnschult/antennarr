import { screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { idleSlot, localSlot, makeState, makeTwoDeviceState, remoteSlot, updateAvailable } from '@/test/fixtures'
import { renderTunerRoutes, stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

const list = (s = makeTwoDeviceState(), path = '/tuners') => renderTunerRoutes(s, path)

describe('the Tuners page', () => {
  it('lists the tuners, each linking to its own page', () => {
    list()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
    const rows = within(screen.getByRole('list', { name: 'Tuners' })).getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(within(rows[0]).getByRole('link', { name: /Steve FLEX 4K/ })).toHaveAttribute('href', '/tuners/antenna')
    expect(within(rows[1]).getByRole('link', { name: /Cable HDHR/ })).toHaveAttribute('href', '/tuners/cable')
  })

  it('shows what a tuner is, where it is and how many of its tuners are in use', () => {
    const s = makeTwoDeviceState()
    s.devices[0].tuner.slots = [localSlot(0), remoteSlot(1), idleSlot(2), idleSlot(3)]
    s.devices[0].label = 'Living room antenna'
    list(s)
    const antenna = screen.getAllByRole('listitem')[0]
    expect(antenna).toHaveTextContent('Living room antenna')
    expect(antenna).toHaveTextContent('Steve FLEX 4K · 192.168.1.132 · port 5004')
    expect(antenna).not.toHaveTextContent('HDFX-4K')
    expect(antenna).toHaveTextContent('2 of 4 in use')
    expect(screen.getAllByRole('listitem')[1]).toHaveTextContent('0 of 2 in use')
  })

  it('does not say the device twice when a tuner has no name of your own', () => {
    list()
    const first = screen.getAllByRole('listitem')[0]
    expect(first.textContent?.match(/Steve FLEX 4K/g)).toHaveLength(1)
  })

  it('flags a tuner with an update, linking to its Firmware tab, and only that one', () => {
    const s = makeTwoDeviceState()
    s.devices[0].firmware = updateAvailable
    list(s)
    const flag = screen.getByRole('link', { name: 'Firmware update' })
    expect(flag).toHaveAttribute('href', '/tuners/antenna/firmware')
    expect(within(screen.getAllByRole('listitem')[1]).queryByText('Firmware update')).not.toBeInTheDocument()
  })

  it('shows a tuner that is restarting or offline by its dot', () => {
    const s = makeTwoDeviceState()
    s.devices[0].tuner = { ...s.devices[0].tuner, restarting: true }
    s.devices[1].tuner = { ...s.devices[1].tuner, online: false }
    list(s)
    expect(within(screen.getAllByRole('listitem')[0]).getByTitle('Restarting')).toHaveClass('animate-pulse')
    expect(within(screen.getAllByRole('listitem')[1]).getByTitle('Offline')).toHaveClass('bg-red-500')
  })

  it('shows the list even for a single tuner, so another can be added', () => {
    list(makeState())
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Tuners')
    expect(screen.getByRole('list', { name: 'Tuners' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeEnabled()
  })

  it('offers to add one, or says there is nowhere to save it', () => {
    const { unmount } = list()
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeEnabled()
    unmount()
    list({ ...makeTwoDeviceState(), editable: false })
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeDisabled()
    expect(screen.getByText(/no settings file to save tuners in/)).toBeInTheDocument()
  })

  it('says there is no tuner yet and looks for one', () => {
    vi.stubGlobal('fetch', stubFetch({}).fn)
    list(makeState({ devices: [] }))
    expect(screen.getByText('No tuner yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add by address' })).toBeInTheDocument()
  })

  it('has one link to each tuner’s page, not two that go to the same place', () => {
    list()
    const hrefs = screen.getAllByRole('link').map((l) => l.getAttribute('href')).filter((h) => h === '/tuners/antenna' || h === '/tuners/cable')
    expect(hrefs).toEqual(['/tuners/antenna', '/tuners/cable'])
  })
})
