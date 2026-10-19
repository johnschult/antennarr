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

  it('says nobody can use the tuner while there are no viewers', () => {
    const s = makeState({ viewers: [] })
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Nobody can use your tuner yet')
    expect(screen.getByRole('link', { name: 'Add a viewer' })).toHaveAttribute('href', '/viewers?add')
  })

  it('says the tuner is offline and what Yagi will do about it', () => {
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
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getAllByRole('status')).toHaveLength(3)
  })

  it('names the offline tuners when there are several devices', () => {
    const s = makeTwoDeviceState()
    s.devices[1].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('A tuner is offline: Cable HDHR.')
    s.devices[0].tuner.online = false
    show(s)
    expect(screen.getAllByRole('status')[1]).toHaveTextContent('2 tuners are offline: Steve FLEX 4K, Cable HDHR.')
  })

  it('talks about tuners in the plural with no viewers and several devices', () => {
    const s = makeTwoDeviceState()
    s.settings.mode = 'no-viewers'
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('Nobody can use your tuners yet.')
  })

  it('names offline tuners by the name the owner gave them', () => {
    const s = makeTwoDeviceState()
    s.devices[1].label = 'Cable box'
    s.devices[1].tuner.online = false
    show(s)
    expect(screen.getByRole('status')).toHaveTextContent('A tuner is offline: Cable box.')
  })
})

describe('the offline banner', () => {
  it('links each offline tuner’s name to its page', () => {
    const state = makeTwoDeviceState()
    state.devices[0].tuner.online = false
    state.devices[1].tuner.online = false
    render(
      <MemoryRouter>
        <Banners state={state} />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: 'Steve FLEX 4K' })).toHaveAttribute('href', '/tuner/antenna')
    expect(screen.getByRole('link', { name: 'Cable HDHR' })).toHaveAttribute('href', '/tuner/cable')
  })

  it('links the one tuner’s offline notice to its page', () => {
    const state = makeState()
    state.devices[0].tuner.online = false
    render(
      <MemoryRouter>
        <Banners state={state} />
      </MemoryRouter>,
    )
    expect(screen.getByRole('link', { name: 'The tuner is offline.' })).toHaveAttribute('href', '/tuner/antenna')
  })
})
