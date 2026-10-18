import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { john, makeState, makeTwoDeviceState, mom } from '@/test/fixtures'
import { mockPhone } from '@/test/match-media'
import { renderWithApp, stubFetch } from '@/test/render'
import { ViewersPage } from './viewers'

afterEach(() => vi.unstubAllGlobals())

const row = (name: string) => screen.getByRole('row', { name: new RegExp(name) })

describe('ViewersPage', () => {
  it('lists viewers with their addresses, tuner use and status', () => {
    renderWithApp(<ViewersPage />, makeState())
    expect(within(row('John')).getByText('100.67.117.32')).toBeInTheDocument()
    expect(within(row('John')).getByText('1/2')).toBeInTheDocument()
    expect(within(row('John')).getByText('Watching')).toBeInTheDocument()
    expect(within(row('Mom')).getByText('10.8.0.0/24')).toBeInTheDocument()
    expect(within(row('Mom')).getByText('Waiting for first contact')).toBeInTheDocument()
  })

  it('tells paused, idle and never-seen viewers apart', () => {
    const s = makeState({
      viewers: [
        { ...john, inUse: 0 },
        { ...mom, limit: 0, name: 'Cabin' },
        { ...mom, name: 'Never' },
      ],
    })
    renderWithApp(<ViewersPage />, s)
    expect(within(row('John')).getByText(/^Seen /)).toBeInTheDocument()
    expect(within(row('Cabin')).getByText('Paused')).toBeInTheDocument()
    expect(within(row('Never')).getByText('Waiting for first contact')).toBeInTheDocument()
  })

  it('invites you to add the first viewer', () => {
    renderWithApp(<ViewersPage />, makeState({ viewers: [] }))
    expect(screen.getByText('No viewers yet')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('disables every change when settings cannot be saved', () => {
    renderWithApp(<ViewersPage />, makeState({ editable: false }))
    expect(screen.getByRole('button', { name: /Add viewer/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Edit John' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Remove John' })).toBeDisabled()
  })

  describe('adding', () => {
    it('sends the form to the server, then goes on to how they connect', async () => {
      const { fn, calls } = stubFetch({
        'POST /viewers': () => ({ status: 201, body: {} }),
        'GET /state': () => ({ body: makeState() }),
      })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), '  Cabin ')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.6{Enter}10.9.0.7, 10.9.1.0/24')
      await user.clear(within(dialog).getByLabelText('Tuners at once'))
      await user.type(within(dialog).getByLabelText('Tuners at once'), '2')
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))

      expect(await within(dialog).findByRole('region', { name: 'The address' })).toBeInTheDocument() // the next step
      expect(calls.find((c) => c.method === 'POST')?.body).toEqual({
        name: 'Cabin',
        addresses: ['10.9.0.6', '10.9.0.7', '10.9.1.0/24'],
        limit: 2,
      })
      expect(await screen.findByText('Added Cabin')).toBeInTheDocument() // the toast
    })

    it('checks the form before bothering the server', async () => {
      const { fn, calls } = stubFetch({})
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      const submit = within(dialog).getByRole('button', { name: 'Next: Connect' })

      await user.click(submit)
      expect(await within(dialog).findByRole('alert')).toHaveTextContent('Give the viewer a name.')

      await user.type(within(dialog).getByLabelText('Name'), 'Cabin')
      await user.click(submit)
      expect(within(dialog).getByRole('alert')).toHaveTextContent('Add at least one address.')

      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.6')
      await user.clear(within(dialog).getByLabelText('Tuners at once'))
      await user.type(within(dialog).getByLabelText('Tuners at once'), '-1')
      await user.click(submit)
      expect(within(dialog).getByRole('alert')).toHaveTextContent('The tuner limit must be 0 or more.')

      expect(calls).toHaveLength(0)
    })

    it("shows the server's reason in the dialog and keeps it open", async () => {
      const { fn } = stubFetch({ 'POST /viewers': () => ({ status: 409, body: { error: 'viewer "Cabin" already exists' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'Cabin')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.6')
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))

      expect(await within(dialog).findByRole('alert')).toHaveTextContent('viewer "Cabin" already exists')
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })

    it('starts each time with a blank form', async () => {
      vi.stubGlobal('fetch', stubFetch({}).fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      await user.type(await screen.findByLabelText('Name'), 'Half-typed')
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      expect(await screen.findByLabelText('Name')).toHaveValue('')
    })
  })

  describe('editing', () => {
    it("fills the form from the viewer and saves under the viewer's current name", async () => {
      const { fn, calls } = stubFetch({
        'PUT /viewers/John': () => ({ body: {} }),
        'GET /state': () => ({ body: makeState() }),
      })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: 'Edit John' }))
      const dialog = await screen.findByRole('dialog')
      expect(within(dialog).getByRole('heading', { name: 'Edit John' })).toBeInTheDocument()
      expect(within(dialog).getByLabelText('Name')).toHaveValue('John')
      expect(within(dialog).getByLabelText('Addresses')).toHaveValue('100.67.117.32')
      expect(within(dialog).getByLabelText('Tuners at once')).toHaveValue(2)

      await user.clear(within(dialog).getByLabelText('Name'))
      await user.type(within(dialog).getByLabelText('Name'), 'Johnny')
      await user.clear(within(dialog).getByLabelText('Tuners at once'))
      await user.type(within(dialog).getByLabelText('Tuners at once'), '0')
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))

      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
      expect(calls.find((c) => c.method === 'PUT')).toMatchObject({
        url: expect.stringContaining('/viewers/John'),
        body: { name: 'Johnny', addresses: ['100.67.117.32'], limit: 0 },
      })
    })
  })

  describe('removing', () => {
    it('asks first, then deletes', async () => {
      const { fn, calls } = stubFetch({
        'DELETE /viewers/Mom': () => ({ status: 204 }),
        'GET /state': () => ({ body: makeState() }),
      })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: 'Remove Mom' }))
      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText('Remove Mom?')).toBeInTheDocument()
      expect(within(dialog).getByText(/no longer be able to use your tuner/)).toBeInTheDocument()
      expect(calls).toHaveLength(0) // nothing yet

      await user.click(within(dialog).getByRole('button', { name: 'Remove' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
      expect(calls.find((c) => c.method === 'DELETE')?.url).toMatch(/\/viewers\/Mom$/)
    })

    it('does nothing if you keep them', async () => {
      const { fn, calls } = stubFetch({})
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: 'Remove Mom' }))
      await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Keep' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
      expect(calls).toHaveLength(0)
    })

    it('warns clearly before removing the last viewer', async () => {
      const { fn, calls } = stubFetch({
        'DELETE /viewers/John': () => ({ status: 204 }),
        'GET /state': () => ({ body: makeState() }),
      })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState({ viewers: [john] }))

      await user.click(screen.getByRole('button', { name: 'Remove John' }))
      const dialog = await screen.findByRole('alertdialog')
      expect(within(dialog).getByText(/nobody can use your tuner/)).toBeInTheDocument()

      await user.click(within(dialog).getByRole('button', { name: 'Remove' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
      expect(calls.find((c) => c.method === 'DELETE')?.url).toMatch(/\/viewers\/John$/)
    })

    it('shows a failure and stays open', async () => {
      const { fn } = stubFetch({ 'DELETE /viewers/Mom': () => ({ status: 404, body: { error: 'no viewer named "Mom"' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeState())

      await user.click(screen.getByRole('button', { name: 'Remove Mom' }))
      const dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Remove' }))
      expect(await within(dialog).findByRole('alert')).toHaveTextContent('no viewer named "Mom"')
      expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    })
  })

  describe('with several devices', () => {
    it("shows a viewer's use on each device, honouring per-device limits", () => {
      renderWithApp(<ViewersPage />, makeTwoDeviceState())
      const john = row('John')
      expect(within(john).getByText('Steve FLEX 4K')).toBeInTheDocument()
      expect(within(john).getByText('1/2')).toBeInTheDocument() // antenna: default 2
      expect(within(john).getByText('0/2')).toBeInTheDocument() // cable: override 2
      expect(within(row('Mom')).getAllByText('0/1')).toHaveLength(2) // limit 1 on each of the two devices
    })

    it('only offers per-device limits when there is more than one device', async () => {
      vi.stubGlobal('fetch', stubFetch({}).fn)
      const user = userEvent.setup()
      const { unmount } = renderWithApp(<ViewersPage />, makeState())
      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      expect(await screen.findByLabelText('Tuners at once')).toBeInTheDocument()
      expect(screen.queryByText(/Per device/)).not.toBeInTheDocument()
      unmount()

      renderWithApp(<ViewersPage />, makeTwoDeviceState())
      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      expect(await screen.findByLabelText('Tuners at once (default)')).toBeInTheDocument()
      expect(screen.getByLabelText('Tuners on Steve FLEX 4K')).toBeInTheDocument()
      expect(screen.getByLabelText('Tuners on Cable HDHR')).toBeInTheDocument()
    })

    it('sends only the per-device limits that were filled in', async () => {
      const { fn, calls } = stubFetch({ 'POST /viewers': () => ({ status: 201, body: {} }), 'GET /state': () => ({ body: makeState() }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeTwoDeviceState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'Cabin')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.6')
      await user.type(within(dialog).getByLabelText('Tuners on Cable HDHR'), '3') // antenna left blank: uses the default
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))

      await waitFor(() => expect(calls.find((c) => c.method === 'POST')).toBeDefined())
      expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Cabin', addresses: ['10.9.0.6'], limit: 1, limits: { cable: 3 } })
    })

    it('treats an explicit 0 as a limit, and a blank box as "use the default"', async () => {
      const { fn, calls } = stubFetch({ 'POST /viewers': () => ({ status: 201, body: {} }), 'GET /state': () => ({ body: makeState() }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeTwoDeviceState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'Guest')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.7')
      await user.type(within(dialog).getByLabelText('Tuners on Steve FLEX 4K'), '0') // paused on the antenna box only
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))

      await waitFor(() => expect(calls.find((c) => c.method === 'POST')).toBeDefined())
      expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ limits: { antenna: 0 } })
    })

    it('prefills overrides when editing, and clears them when emptied', async () => {
      const { fn, calls } = stubFetch({ 'PUT /viewers/John': () => ({ body: {} }), 'GET /state': () => ({ body: makeState() }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeTwoDeviceState())

      await user.click(screen.getByRole('button', { name: 'Edit John' }))
      const dialog = await screen.findByRole('dialog')
      expect(within(dialog).getByLabelText('Tuners on Cable HDHR')).toHaveValue(2) // John's override
      expect(within(dialog).getByLabelText('Tuners on Steve FLEX 4K')).toHaveValue(null) // none: default

      await user.clear(within(dialog).getByLabelText('Tuners on Cable HDHR'))
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(calls.find((c) => c.method === 'PUT')).toBeDefined())
      expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ name: 'John', addresses: ['100.67.117.32'], limit: 2, limits: {} })
    })

    it('rejects a bad per-device limit, naming the device', async () => {
      const { fn, calls } = stubFetch({})
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, makeTwoDeviceState())

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'X')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.8')
      await user.type(within(dialog).getByLabelText('Tuners on Cable HDHR'), '-2')
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))
      expect(await within(dialog).findByRole('alert')).toHaveTextContent('The limit for Cable HDHR must be 0 or more.')
      expect(calls).toHaveLength(0)
    })

    it('shows a viewer as paused only when every device is paused', () => {
      const s = makeTwoDeviceState()
      s.viewers = [
        { ...mom, name: 'OnePaused', limit: 0, limits: { cable: 1 } }, // paused on antenna, allowed on cable
        { ...mom, name: 'AllPaused', limit: 0 },
      ]
      renderWithApp(<ViewersPage />, s)
      expect(within(row('AllPaused')).getByText('Paused')).toBeInTheDocument()
      expect(within(row('OnePaused')).queryByText('Paused')).not.toBeInTheDocument()
    })
  })

  describe('with names the owner chose', () => {
    it('labels each device by its name in the table and in the dialog', async () => {
      vi.stubGlobal('fetch', stubFetch({}).fn)
      const s = makeTwoDeviceState()
      s.devices[1].label = 'Cable box'
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, s)

      expect(within(row('John')).getByText('Cable box')).toBeInTheDocument()
      expect(within(row('John')).getByText('Steve FLEX 4K')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Edit John' }))
      const dialog = await screen.findByRole('dialog')
      expect(within(dialog).getByLabelText('Tuners on Cable box')).toHaveValue(2)
      // ...but limits are still saved under the device's key, which never changes.
      expect(within(dialog).getByLabelText('Tuners on Steve FLEX 4K')).toBeInTheDocument()
    })

    it('saves limits under the key, whatever the tuner is called', async () => {
      const { fn, calls } = stubFetch({ 'POST /viewers': () => ({ status: 201, body: {} }), 'GET /state': () => ({ body: makeState() }) })
      vi.stubGlobal('fetch', fn)
      const s = makeTwoDeviceState()
      s.devices[1].label = 'Cable box'
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, s)

      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'Guest')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.9')
      await user.type(within(dialog).getByLabelText('Tuners on Cable box'), '3')
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))
      await waitFor(() => expect(calls.find((c) => c.method === 'POST')).toBeDefined())
      expect(calls.find((c) => c.method === 'POST')?.body).toMatchObject({ limits: { cable: 3 } })
    })

    it('uses the name in the per-device error', async () => {
      vi.stubGlobal('fetch', stubFetch({}).fn)
      const s = makeTwoDeviceState()
      s.devices[1].label = 'Cable box'
      const user = userEvent.setup()
      renderWithApp(<ViewersPage />, s)
      await user.click(screen.getByRole('button', { name: /Add viewer/ }))
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText('Name'), 'X')
      await user.type(within(dialog).getByLabelText('Addresses'), '10.9.0.8')
      await user.type(within(dialog).getByLabelText('Tuners on Cable box'), '-2')
      await user.click(within(dialog).getByRole('button', { name: 'Next: Connect' }))
      expect(await within(dialog).findByRole('alert')).toHaveTextContent('The limit for Cable box must be 0 or more.')
    })
  })
})

describe('ViewersPage on a phone', () => {
  const card = (name: string) => within(screen.getByRole('list', { name: 'Viewers' })).getByText(name).closest('li')!

  it('shows each viewer as a card instead of a table that would scroll sideways', () => {
    mockPhone(true)
    renderWithApp(<ViewersPage />, makeState())
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(within(screen.getByRole('list', { name: 'Viewers' })).getAllByRole('listitem')).toHaveLength(2)
  })

  it('carries the same details as the table: addresses, tuner use and status', () => {
    mockPhone(true)
    renderWithApp(<ViewersPage />, makeState())
    expect(within(card('John')).getByText('100.67.117.32')).toBeInTheDocument()
    expect(within(card('John')).getByText('1/2')).toBeInTheDocument()
    expect(within(card('John')).getByText('Watching')).toBeInTheDocument()
    expect(within(card('Mom')).getByText('10.8.0.0/24')).toBeInTheDocument()
    expect(within(card('Mom')).getByText('Waiting for first contact')).toBeInTheDocument()
  })

  it('lets long names and many addresses wrap instead of pushing the page wider', () => {
    mockPhone(true)
    const long = { ...mom, name: 'A viewer with a rather long name', addresses: ['10.8.0.0/24', '100.64.0.9', 'fd7a:115c:a1e0::1234:5678'] }
    renderWithApp(<ViewersPage />, makeState({ viewers: [long] }))
    const addresses = within(card(long.name)).getByText(long.addresses.join(', '))
    expect(addresses.className).toMatch(/break-all/)
  })

  it('edits and removes from the card, and disables both when the settings cannot be saved', async () => {
    mockPhone(true)
    const user = userEvent.setup()
    const { unmount } = renderWithApp(<ViewersPage />, makeState())
    await user.click(within(card('John')).getByRole('button', { name: 'Edit John' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    unmount()

    const s = makeState()
    s.editable = false
    renderWithApp(<ViewersPage />, s)
    expect(within(card('John')).getByRole('button', { name: 'Edit John' })).toBeDisabled()
    expect(within(card('John')).getByRole('button', { name: 'Remove John' })).toBeDisabled()
  })

  it('shows each tuner of a viewer on a device with several', () => {
    mockPhone(true)
    renderWithApp(<ViewersPage />, makeTwoDeviceState())
    expect(within(card('John')).getByText('Steve FLEX 4K')).toBeInTheDocument()
    expect(within(card('John')).getByText('Cable HDHR')).toBeInTheDocument()
  })

  it('switches between the card list and the table as the window is resized', () => {
    const viewport = mockPhone(true)
    renderWithApp(<ViewersPage />, makeState())
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    act(() => viewport.set(false))
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Viewers' })).not.toBeInTheDocument()
    act(() => viewport.set(true))
    expect(screen.getByRole('list', { name: 'Viewers' })).toBeInTheDocument()
  })

  it('still says so when there are no viewers', () => {
    mockPhone(true)
    renderWithApp(<ViewersPage />, makeState({ viewers: [] }))
    expect(screen.getByText('No viewers yet')).toBeInTheDocument()
  })
})

describe('ViewersPage: the connection steps', () => {
  it('opens how to connect for the viewer in that row, without changing them', async () => {
    const { fn, calls } = stubFetch({ 'GET /network': () => ({ body: { addresses: [] } }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, makeState())

    await user.click(within(row('Mom')).getByRole('button', { name: 'Connection steps for Mom' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Connect Mom' })).toBeInTheDocument()
    expect(await within(dialog).findByRole('region', { name: 'The address' })).toBeInTheDocument()
    expect(calls.some((c) => c.method !== 'GET')).toBe(false)
  })

  it('is on the phone layout too', async () => {
    mockPhone(true)
    vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses: [] } }) }).fn)
    renderWithApp(<ViewersPage />, makeState())
    await userEvent.setup().click(screen.getAllByRole('button', { name: 'Connection steps for John' })[0])
    expect(await screen.findByRole('heading', { name: 'Connect John' })).toBeInTheDocument()
  })

  it('opens the add-viewer wizard from a link with ?add', async () => {
    renderWithApp(<ViewersPage />, makeState({ viewers: [] }), { route: '/viewers?add' })
    expect(await screen.findByRole('heading', { name: 'Add a viewer' })).toBeInTheDocument()
  })

  it('does not open from ?add when settings cannot be saved', () => {
    renderWithApp(<ViewersPage />, makeState({ editable: false }), { route: '/viewers?add' })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('has an Add viewer button in the empty state too', async () => {
    renderWithApp(<ViewersPage />, makeState({ viewers: [] }))
    const empty = screen.getByText('No viewers yet').parentElement!
    await userEvent.setup().click(within(empty).getByRole('button', { name: /Add viewer/ }))
    expect(await screen.findByRole('heading', { name: 'Add a viewer' })).toBeInTheDocument()
  })

  it('still edits a viewer in the single form', async () => {
    renderWithApp(<ViewersPage />, makeState())
    await userEvent.setup().click(screen.getByRole('button', { name: 'Edit John' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Edit John' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('list', { name: 'Steps' })).not.toBeInTheDocument()
  })
})
