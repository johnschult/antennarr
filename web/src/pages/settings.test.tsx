import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { SettingsPage } from './settings'

afterEach(() => vi.unstubAllGlobals())

const state = () => {
  const s = makeState()
  s.settings = { mode: 'enforced', sharedCap: 0, ownerReserve: 1, admin: ['100.64.0.0/10'] }
  s.devices[0].ownerReserve = 1
  s.devices[0].sharedCap = 2
  return s
}
const saveFor = (title: string) => screen.getByText(title).closest('[data-slot="card"]')!.querySelector('button[type="submit"]') as HTMLElement

describe('SettingsPage', () => {
  it('shows the current settings', () => {
    renderWithApp(<SettingsPage />, state())
    expect(screen.getByLabelText('Tuners to keep free')).toHaveValue(1)
    expect(screen.getByLabelText('Tuners for viewers')).toHaveValue(2)
    expect(screen.getByLabelText('Admin addresses')).toHaveValue('100.64.0.0/10')
    expect(screen.getByText('Your tuner has 4.')).toBeInTheDocument()
    expect(screen.getByText('viewers only')).toBeInTheDocument()
  })

  it('says when access is open', () => {
    const s = state()
    s.settings.mode = 'open'
    renderWithApp(<SettingsPage />, s)
    expect(screen.getByText('open (no viewers)')).toBeInTheDocument()
  })

  it('saves only the field that was changed', async () => {
    const { fn, calls } = stubFetch({
      'PATCH /devices/antenna': () => ({ body: {} }),
      'GET /state': () => ({ body: makeState() }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    await user.clear(screen.getByLabelText('Tuners to keep free'))
    await user.type(screen.getByLabelText('Tuners to keep free'), '2')
    await user.click(saveFor('Owner reserve'))

    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ ownerReserve: 2 })
    expect(await screen.findByText('Settings saved')).toBeInTheDocument()
  })

  it('sends zero when the cap is cleared to 0, which means "no cap"', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    await user.clear(screen.getByLabelText('Tuners for viewers'))
    await user.type(screen.getByLabelText('Tuners for viewers'), '0')
    await user.click(saveFor('Shared cap'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ sharedCap: 0 }))
  })

  it('parses the admin list into addresses', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    const box = screen.getByLabelText('Admin addresses')
    await user.clear(box)
    await user.type(box, '192.168.1.5{Enter}10.0.0.0/8')
    await user.click(saveFor('Admin access'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ admin: ['192.168.1.5', '10.0.0.0/8'] }))
  })

  it('can empty the admin list', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    await user.clear(screen.getByLabelText('Admin addresses'))
    await user.click(saveFor('Admin access'))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ admin: [] }))
  })

  it('rejects nonsense locally', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    for (const bad of ['-1', '1.5', '']) { // '' is an emptied box, which must not quietly become 0
      const input = screen.getByLabelText('Tuners to keep free')
      await user.clear(input)
      if (bad) await user.type(input, bad)
      await user.click(saveFor('Owner reserve'))
      expect(await screen.findByText('Enter a whole number, 0 or more.')).toBeInTheDocument()
    }
    expect(calls).toHaveLength(0)
  })

  it("shows the server's reason when it refuses a value", async () => {
    const { fn } = stubFetch({ 'PATCH /settings': () => ({ status: 400, body: { error: 'admin: bad address "nope"' } }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, state())

    await user.clear(screen.getByLabelText('Admin addresses'))
    await user.type(screen.getByLabelText('Admin addresses'), 'nope')
    await user.click(saveFor('Admin access'))
    expect(await screen.findByRole('alert')).toHaveTextContent('admin: bad address "nope"')
  })

  it('cannot be edited when there is no settings file', () => {
    renderWithApp(<SettingsPage />, { ...state(), editable: false })
    expect(screen.getByLabelText('Tuners to keep free')).toBeDisabled()
    expect(screen.getByLabelText('Tuners for viewers')).toBeDisabled()
    expect(screen.getByLabelText('Admin addresses')).toBeDisabled()
    for (const t of ['Owner reserve', 'Shared cap', 'Admin access']) expect(saveFor(t)).toBeDisabled()
  })

  describe('with several devices', () => {
    const card = (title: string) => screen.getByText(title).closest('[data-slot="card"]') as HTMLElement

    it('has an owner reserve and a shared cap for each device, with that device\'s values', () => {
      const s = makeTwoDeviceState()
      s.devices[0].sharedCap = 2
      renderWithApp(<SettingsPage />, s)
      expect(within(card('Owner reserve — antenna')).getByLabelText('Tuners to keep free')).toHaveValue(1)
      expect(within(card('Owner reserve — cable')).getByLabelText('Tuners to keep free')).toHaveValue(0)
      expect(within(card('Shared cap — antenna')).getByLabelText('Tuners for viewers')).toHaveValue(2)
      expect(within(card('Shared cap — cable')).getByLabelText('Tuners for viewers')).toHaveValue(0)
      expect(screen.getByText('cable has 2.')).toBeInTheDocument()
    })

    it("saves a device's value on that device only", async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/cable': () => ({ body: {} }), 'GET /state': () => ({ body: makeState() }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, makeTwoDeviceState())

      const box = within(card('Owner reserve — cable')).getByLabelText('Tuners to keep free')
      await user.clear(box)
      await user.type(box, '1')
      await user.click(within(card('Owner reserve — cable')).getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/cable'), body: { ownerReserve: 1 } })
    })

    it('lists where each device listens', () => {
      renderWithApp(<SettingsPage />, makeTwoDeviceState())
      expect(screen.getByText('antenna listens on :5004')).toBeInTheDocument()
      expect(screen.getByText('cable listens on :5005')).toBeInTheDocument()
    })
  })
})
