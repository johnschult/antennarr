import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { Banners } from './banners'

const show = (s = makeState()) =>
  render(
    <MemoryRouter>
      <Banners state={s} />
    </MemoryRouter>,
  )

describe('Banners', () => {
  it('shows nothing when all is well', () => {
    show()
    expect(screen.queryAllByRole('status')).toHaveLength(0)
  })

  it('warns that the tuner is open to everyone while there are no viewers', () => {
    const s = makeState({ viewers: [] })
    s.settings.mode = 'open'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Anyone who can reach this port can use your tuner')
    expect(screen.getByRole('link', { name: 'Add a viewer' })).toHaveAttribute('href', '/viewers')
  })

  it('says the tuner is offline and what Antennarr will do about it', () => {
    const s = makeState()
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('The tuner is offline')
    expect(screen.getByRole('status')).toHaveTextContent('10ABCC87')
  })

  it("explains why changes can't be saved when there is no settings file", () => {
    show(makeState({ editable: false }))
    expect(screen.getByRole('status')).toHaveTextContent('-config')
  })

  it('can show several at once', () => {
    const s = makeState({ editable: false })
    s.devices[0].tuner.online = false
    s.settings.mode = 'open'
    show(s)
    expect(screen.getAllByRole('status')).toHaveLength(3)
  })

  it('names the offline tuners when there are several devices', () => {
    const s = makeTwoDeviceState()
    s.devices[1].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('A tuner is offline: cable.')
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getAllByRole('status')[1]).toHaveTextContent('2 tuners are offline: antenna, cable.')
  })

  it('talks about ports and tuners in the plural when open with several devices', () => {
    const s = makeTwoDeviceState()
    s.settings.mode = 'open'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Anyone who can reach these ports can use your tuners.')
  })
})
