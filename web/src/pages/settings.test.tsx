import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fakeWindow, stubOpen } from '@/test/fake-window'
import { makeDevice, makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { SettingsPage } from './settings'

afterEach(() => vi.unstubAllGlobals())

const state = () => {
  const s = makeState()
  s.settings = { mode: 'enforced', sharedCap: 0, ownerReserve: 1, admin: ['100.64.0.0/10'], firmwareCheck: true, guide: true, plex: { enabled: false, admins: [] } }
  s.devices[0].ownerReserve = 1
  s.devices[0].sharedCap = 2
  return s
}
const section = (name: string) => screen.getByRole('region', { name })
const saveButton = (scope: HTMLElement) => within(scope).queryByRole('button', { name: 'Save' })
const okState = { 'GET /state': () => ({ body: makeState() }) }

describe('SettingsPage', () => {
  it('groups settings into sections of rows, not a card each', () => {
    renderWithApp(<SettingsPage />, state())
    for (const name of ['Tuner', 'Access', 'Updates', 'Network']) expect(section(name)).toBeInTheDocument()
    expect(screen.queryByText(/^Save$/)).not.toBeInTheDocument() // nothing to save until something changes
    expect(document.querySelector('[data-slot="card"]')).toBeNull()
  })

  it("shows each setting's current value", () => {
    renderWithApp(<SettingsPage />, state())
    const tuner = section('Tuner')
    expect(within(tuner).getByLabelText('Name')).toHaveValue('')
    expect(within(tuner).getByLabelText('Name')).toHaveAttribute('placeholder', 'Steve FLEX 4K') // the tuner's own name, until you choose one
    expect(within(tuner).getByLabelText('Keep free for you')).toHaveValue(1)
    expect(within(tuner).getByLabelText('Most for viewers')).toHaveValue(2)
    expect(within(tuner).getByText(/4 tuners · listening on :5004/)).toBeInTheDocument()
    expect(screen.getByLabelText('Admin addresses')).toHaveValue('100.64.0.0/10')
    expect(screen.getByRole('switch', { name: 'Check for new tuner firmware' })).toBeChecked()
    expect(within(section('Access')).getByText('Viewers only')).toBeInTheDocument()
  })

  it('says when there are no viewers yet', () => {
    const s = state()
    s.settings.mode = 'no-viewers'
    renderWithApp(<SettingsPage />, s)
    expect(within(section('Access')).getByText('No viewers yet')).toBeInTheDocument()
  })

  describe('naming a tuner', () => {
    it('saves the name, trimmed, on that tuner', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())

      const box = screen.getByLabelText('Name')
      expect(saveButton(section('Tuner'))).toBeNull()
      await user.type(box, '  Antenna (Steve)  ')
      await user.click(saveButton(section('Tuner'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/antenna'), body: { label: 'Antenna (Steve)' } })
      expect(await screen.findByText('Settings saved')).toBeInTheDocument()
    })

    it('saves with Enter', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.type(screen.getByLabelText('Name'), 'Antenna{Enter}')
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: 'Antenna' }))
    })

    it('can be cleared to go back to the tuner’s own name', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      const s = state()
      s.devices[0].label = 'Antenna (Steve)'
      renderWithApp(<SettingsPage />, s)

      expect(screen.getByLabelText('Name')).toHaveValue('Antenna (Steve)')
      await user.clear(screen.getByLabelText('Name'))
      await user.click(saveButton(section('Antenna (Steve)'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ label: '' }))
    })

    it('stops at 60 characters, the most the server accepts', () => {
      renderWithApp(<SettingsPage />, state())
      expect(screen.getByLabelText('Name')).toHaveAttribute('maxlength', '60')
    })

    it("shows the server's objection on the row", async () => {
      const { fn } = stubFetch({ 'PATCH /devices/antenna': () => ({ status: 400, body: { error: 'name for "antenna": can\'t contain control characters' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.type(screen.getByLabelText('Name'), 'x')
      await user.click(saveButton(section('Tuner'))!)
      expect(await within(section('Tuner')).findByRole('alert')).toHaveTextContent('control characters')
    })
  })

  describe('numbers', () => {
    it('offers Save and Reset only once a value has changed, and Reset puts it back', async () => {
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      const box = screen.getByLabelText('Keep free for you')
      expect(saveButton(section('Tuner'))).toBeNull()

      await user.clear(box)
      await user.type(box, '2')
      expect(saveButton(section('Tuner'))).toBeInTheDocument()
      await user.click(within(section('Tuner')).getByRole('button', { name: 'Reset' }))
      expect(box).toHaveValue(1)
      expect(saveButton(section('Tuner'))).toBeNull()

      await user.clear(box)
      await user.type(box, '1') // typed back to the saved value: nothing to save
      expect(saveButton(section('Tuner'))).toBeNull()
    })

    it('saves only the field that was changed, on the device', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())

      const box = screen.getByLabelText('Keep free for you')
      await user.clear(box)
      await user.type(box, '2')
      await user.click(saveButton(section('Tuner'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ ownerReserve: 2 })
    })

    it('sends zero for "no cap"', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.clear(screen.getByLabelText('Most for viewers'))
      await user.type(screen.getByLabelText('Most for viewers'), '0')
      await user.click(saveButton(section('Tuner'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ sharedCap: 0 }))
    })

    it('rejects nonsense locally, including an emptied box that would quietly become 0', async () => {
      const { fn, calls } = stubFetch({})
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())

      for (const bad of ['-1', '1.5', '']) {
        const input = screen.getByLabelText('Keep free for you')
        await user.clear(input)
        if (bad) await user.type(input, bad)
        await user.click(saveButton(section('Tuner'))!)
        expect(await within(section('Tuner')).findByText('Enter a whole number, 0 or more.')).toBeInTheDocument()
      }
      expect(calls).toHaveLength(0)
    })
  })

  describe('access', () => {
    it('parses the admin list into addresses', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())

      const box = screen.getByLabelText('Admin addresses')
      await user.clear(box)
      await user.type(box, '192.168.1.5{Enter}10.0.0.0/8') // Enter makes a new line in a multi-line box
      await user.click(saveButton(section('Access'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ admin: ['192.168.1.5', '10.0.0.0/8'] }))
    })

    it('can empty the admin list', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.clear(screen.getByLabelText('Admin addresses'))
      await user.click(saveButton(section('Access'))!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ admin: [] }))
    })

    it("shows the server's reason when it refuses an address", async () => {
      const { fn } = stubFetch({ 'PATCH /settings': () => ({ status: 400, body: { error: 'admin: bad address "nope"' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.clear(screen.getByLabelText('Admin addresses'))
      await user.type(screen.getByLabelText('Admin addresses'), 'nope')
      await user.click(saveButton(section('Access'))!)
      expect(await within(section('Access')).findByRole('alert')).toHaveTextContent('admin: bad address "nope"')
    })
  })

  describe('firmware check', () => {
    it('is a switch that takes effect when flipped', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())

      await user.click(screen.getByRole('switch', { name: 'Check for new tuner firmware' }))
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ firmwareCheck: false })
    })

    it('shows it off, and turns it on', async () => {
      const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      const s = state()
      s.settings.firmwareCheck = false
      renderWithApp(<SettingsPage />, s)

      expect(screen.getByRole('switch', { name: 'Check for new tuner firmware' })).not.toBeChecked()
      await user.click(screen.getByRole('switch', { name: 'Check for new tuner firmware' }))
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ firmwareCheck: true }))
    })

    it('says what it sends: nothing about you', () => {
      renderWithApp(<SettingsPage />, state())
      expect(within(section('Updates')).getByText(/Nothing about you or your tuners is sent/)).toBeInTheDocument()
      expect(within(section('Updates')).getByText(/never installs them/)).toBeInTheDocument()
    })

    it('shows a failure on the row', async () => {
      const { fn } = stubFetch({ 'PATCH /settings': () => ({ status: 500, body: { error: 'could not save the settings' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, state())
      await user.click(screen.getByRole('switch', { name: 'Check for new tuner firmware' }))
      expect(await within(section('Updates')).findByRole('alert')).toHaveTextContent('could not save the settings')
    })
  })

  it('cannot be edited when there is no settings file', async () => {
    const user = userEvent.setup()
    renderWithApp(<SettingsPage />, { ...state(), editable: false })
    for (const label of ['Name', 'Keep free for you', 'Most for viewers', 'Admin addresses']) expect(screen.getByLabelText(label)).toBeDisabled()
    expect(screen.getByRole('switch', { name: 'Check for new tuner firmware' })).toHaveAttribute('aria-disabled', 'true')
    await user.type(screen.getByLabelText('Name'), 'x') // typing into a disabled box does nothing
    expect(saveButton(section('Tuner'))).toBeNull()
  })

  describe('with several devices', () => {
    it("has a section for each, titled with the name it was given, each showing its own values", () => {
      const s = makeTwoDeviceState()
      s.devices[0].sharedCap = 2
      s.devices[1] = makeDevice({ ...s.devices[1], label: 'Cable box' })
      renderWithApp(<SettingsPage />, s)

      const antenna = section('antenna')
      const cable = section('Cable box')
      expect(within(antenna).getByLabelText('Keep free for you')).toHaveValue(1)
      expect(within(antenna).getByLabelText('Most for viewers')).toHaveValue(2)
      expect(within(cable).getByLabelText('Keep free for you')).toHaveValue(0)
      expect(within(cable).getByLabelText('Name')).toHaveValue('Cable box')
      expect(within(cable).getByText(/Cable HDHR · 2 tuners · listening on :5005/)).toBeInTheDocument()
    })

    it("saves a device's value on that device only", async () => {
      const { fn, calls } = stubFetch({ 'PATCH /devices/cable': () => ({ body: {} }), ...okState })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<SettingsPage />, makeTwoDeviceState())

      const cable = section('cable')
      const box = within(cable).getByLabelText('Keep free for you')
      await user.clear(box)
      await user.type(box, '1')
      await user.click(saveButton(cable)!)
      await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')).toBeDefined())
      expect(calls.find((c) => c.method === 'PATCH')).toMatchObject({ url: expect.stringContaining('/devices/cable'), body: { ownerReserve: 1 } })
    })

    it('lists where each device listens, by name', () => {
      const s = makeTwoDeviceState()
      s.devices[1].label = 'Cable box'
      renderWithApp(<SettingsPage />, s)
      const network = section('Network')
      expect(within(network).getByText('antenna')).toBeInTheDocument()
      expect(within(network).getByText('Cable box')).toBeInTheDocument()
      expect(within(network).getByText(':5004')).toBeInTheDocument()
      expect(within(network).getByText(':5005')).toBeInTheDocument()
    })
  })

  it('titles the section of a single named tuner with its name', () => {
    const s = state()
    s.devices[0].label = 'Living room antenna'
    renderWithApp(<SettingsPage />, s)
    expect(section('Living room antenna')).toBeInTheDocument()
  })
})

describe('guide setting', () => {
  it('is on by default and can be switched off, saying what it sends', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<SettingsPage />, state())
    const sw = within(section('Updates')).getByRole('switch', { name: "Show what's on" })
    expect(sw).toBeChecked()
    expect(within(section('Updates')).getByText(/sends SiliconDust your tuner's guide key/)).toBeInTheDocument()
    await userEvent.setup().click(sw)
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ guide: false }))
  })

  it('cannot be changed from a read-only view', () => {
    const s = state()
    s.editable = false
    renderWithApp(<SettingsPage />, s)
    expect(within(section('Updates')).getByRole('switch', { name: "Show what's on" })).toHaveAttribute('aria-disabled', 'true')
  })
})

describe('Plex sign-in settings', () => {
  const withPlex = (admins: { id: number; name: string }[] = [], enabled = true) => {
    const s = state()
    s.settings.plex = { enabled, admins }
    return s
  }
  const access = () => section('Access')

  it('is off by default and shows no accounts list', () => {
    renderWithApp(<SettingsPage />, withPlex([], false))
    expect(within(access()).getByRole('switch', { name: 'Sign in with Plex' })).not.toBeChecked()
    expect(screen.queryByText('Allowed Plex accounts')).not.toBeInTheDocument()
  })

  it('turns on with the switch', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<SettingsPage />, withPlex([], false))
    await userEvent.setup().click(within(access()).getByRole('switch', { name: 'Sign in with Plex' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ plexEnabled: true }))
  })

  it('lists the allowed accounts, or says there are none', () => {
    const { unmount } = renderWithApp(<SettingsPage />, withPlex())
    expect(screen.getByText('No accounts yet.')).toBeInTheDocument()
    unmount()
    renderWithApp(<SettingsPage />, withPlex([{ id: 1, name: 'john' }, { id: 2, name: 'steve' }]))
    expect(screen.getByText('john')).toBeInTheDocument()
    expect(screen.getByText('steve')).toBeInTheDocument()
    expect(screen.queryByText('No accounts yet.')).not.toBeInTheDocument()
  })

  it('removes an account', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /plex/admins/2': () => ({ status: 204 }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<SettingsPage />, withPlex([{ id: 1, name: 'john' }, { id: 2, name: 'steve' }]))
    await userEvent.setup().click(screen.getByRole('button', { name: 'Remove steve' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/plex/admins/2'))).toBe(true))
    expect(await screen.findByText('steve can no longer sign in')).toBeInTheDocument()
  })

  it('shows why removing failed', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'DELETE /plex/admins/1': () => ({ status: 409, body: { error: 'that is the last admin' } }) }).fn)
    renderWithApp(<SettingsPage />, withPlex([{ id: 1, name: 'john' }]))
    await userEvent.setup().click(screen.getByRole('button', { name: 'Remove john' }))
    expect(await screen.findByText('that is the last admin')).toBeInTheDocument()
  })

  it('adds an account through a Plex sign-in', async () => {
    const { fn, calls } = stubFetch({
      'POST /auth/plex/add/start': () => ({ body: { pinId: 9, authUrl: 'https://app.plex.tv/auth#?code=z' } }),
      'POST /auth/plex/add/poll': () => ({ body: { status: 'ok', account: { id: 5, name: 'newbie' } } }),
      ...okState,
    })
    vi.stubGlobal('fetch', fn)
    const win = fakeWindow()
    stubOpen(win)
    renderWithApp(<SettingsPage />, withPlex())
    await userEvent.setup().click(screen.getByRole('button', { name: 'Add a Plex account' }))
    expect(win.location.href).toBe('https://app.plex.tv/auth#?code=z')
    expect(await screen.findByText('newbie can now sign in.', {}, { timeout: 6000 })).toBeInTheDocument()
    expect(calls.map((c) => c.url.replace(/^.*?(?=\/auth\/)/, ''))).toContain('/auth/plex/add/poll')
  }, 15000)

  it('cannot be changed from a read-only view', () => {
    const s = withPlex([{ id: 1, name: 'john' }])
    s.editable = false
    renderWithApp(<SettingsPage />, s)
    expect(screen.getByRole('button', { name: 'Remove john' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Add a Plex account' })).toBeDisabled()
    expect(within(access()).getByRole('switch', { name: 'Sign in with Plex' })).toHaveAttribute('aria-disabled', 'true')
  })
})
