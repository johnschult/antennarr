import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SettingsPage } from '@/pages/settings'
import { discord, makeState, phone, pushover, withChannels } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'

afterEach(() => {
  vi.unstubAllGlobals()
  toast.dismiss() // the toast store is shared by every test in the file
})

type State = ReturnType<typeof makeState>
const section = () => screen.getByRole('region', { name: 'Notifications' })
const show = (state: State = makeState()) => renderWithApp(<SettingsPage />, state)
const ok = { 'GET /state': () => ({ body: makeState() }) }

describe('Notifications: what sends a message', () => {
  it('has a switch for each kind of event, off for unknown addresses until you say', () => {
    show()
    const on = (name: RegExp) => within(section()).getByRole('switch', { name })
    expect(on(/New tuner firmware/)).toBeChecked()
    expect(on(/goes offline/)).toBeChecked()
    expect(on(/turned away/)).toBeChecked()
    expect(on(/is not a viewer connects/)).not.toBeChecked()
  })

  it('switches one event and leaves the others alone', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /settings': () => ({ body: {} }), ...ok })
    vi.stubGlobal('fetch', fn)
    show()
    await userEvent.setup().click(within(section()).getByRole('switch', { name: /is not a viewer connects/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ notifyEvents: { unknown: true } })
  })

  it('cannot switch on firmware news while the firmware check is off, and says what to do', () => {
    const s = makeState()
    s.settings.firmwareCheck = false
    show(s)
    const sw = within(section()).getByRole('switch', { name: /New tuner firmware/ })
    expect(sw).not.toBeChecked() // even though the setting is on, it can't do anything now
    expect(sw).toHaveAttribute('aria-disabled', 'true')
    expect(within(section()).getByText(/Turn on “Check for new tuner firmware” under Privacy and updates first/)).toBeInTheDocument()
    // the other events are not held up by it
    expect(within(section()).getByRole('switch', { name: /goes offline/ })).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('does not even try to send it', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    const s = makeState()
    s.settings.firmwareCheck = false
    show(s)
    await userEvent.setup().click(within(section()).getByRole('switch', { name: /New tuner firmware/ }))
    expect(calls).toEqual([])
  })

  it('is as before once the firmware check is on', () => {
    show()
    const sw = within(section()).getByRole('switch', { name: /New tuner firmware/ })
    expect(sw).toBeChecked()
    expect(sw).not.toHaveAttribute('aria-disabled', 'true')
    expect(within(section()).queryByText(/Turn on “Check for new tuner firmware”/)).not.toBeInTheDocument()
  })

  it('cannot be changed when settings cannot be saved', () => {
    show(withChannels(makeState({ editable: false }), phone))
    for (const sw of within(section()).getAllByRole('switch')) expect(sw).toHaveAttribute('aria-disabled', 'true')
    expect(within(section()).getByRole('button', { name: /Add channel/ })).toBeDisabled()
    expect(within(section()).getByRole('button', { name: 'Send a test to Phone' })).toBeDisabled()
    expect(within(section()).getByRole('button', { name: 'Edit Phone' })).toBeDisabled()
    expect(within(section()).getByRole('button', { name: 'Remove Phone' })).toBeDisabled()
  })
})

describe('Notifications: where it goes', () => {
  it('says nothing is sent until a channel is added', () => {
    show()
    expect(within(section()).getByText('Where to send them')).toBeInTheDocument()
    expect(within(section()).getByText(/Nothing is sent, and nothing is contacted/)).toBeInTheDocument()
  })

  it('lists each channel with where it goes, and never anything secret', () => {
    show(withChannels(makeState(), phone, pushover, discord))
    const sec = within(section())
    expect(sec.getByText('ntfy · ntfy.sh/antennarr')).toBeInTheDocument()
    expect(sec.getAllByText('Pushover')).toHaveLength(2) // its name, and what it is
    expect(sec.getByText('Webhook · discord.com')).toBeInTheDocument()
    expect(sec.getByText('Add another')).toBeInTheDocument()
  })

  it('stops offering to add at eight channels', () => {
    const many = Array.from({ length: 8 }, (_, i) => ({ ...phone, id: `c${i}`, name: `Phone ${i}` }))
    show(withChannels(makeState(), ...many))
    expect(within(section()).getByRole('button', { name: /Add channel/ })).toBeDisabled()
  })
})

describe('Notifications: the Test button on a channel', () => {
  it('is disabled and says it is sending until the answer is in, then says how it went in one toast', async () => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => (release = resolve))
    const { fn, calls } = stubFetch({ 'POST /notifications/test': () => ({ body: { ok: true } }) })
    vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') await gate
      return fn(input, init)
    })
    const user = userEvent.setup()
    show(withChannels(makeState(), phone, pushover))

    await user.click(within(section()).getByRole('button', { name: 'Send a test to Phone' }))
    const busy = await within(section()).findByRole('button', { name: 'Send a test to Phone' })
    expect(busy).toBeDisabled()
    expect(busy).toHaveTextContent('Sending…')
    // only that row is busy
    expect(within(section()).getByRole('button', { name: 'Send a test to Pushover' })).toBeEnabled()
    expect(screen.queryByText(/sending/i, { selector: '[data-sonner-toast] *' })).not.toBeInTheDocument()

    release()
    expect(await screen.findByText('Test sent to Phone')).toBeInTheDocument()
    expect(within(section()).getByRole('button', { name: 'Send a test to Phone' })).toBeEnabled()
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ id: 'c1' })
  })

  it("shows the server's reason when it did not go through", async () => {
    vi.stubGlobal('fetch', stubFetch({ 'POST /notifications/test': () => ({ status: 502, body: { error: 'the test to "Phone" didn\'t go through: ntfy.sh answered 401 Unauthorized' } }) }).fn)
    show(withChannels(makeState(), phone))
    await userEvent.setup().click(within(section()).getByRole('button', { name: 'Send a test to Phone' }))
    expect(await screen.findByText(/ntfy.sh answered 401/)).toBeInTheDocument()
    expect(screen.queryByText('Test sent to Phone')).not.toBeInTheDocument()
  })
})

describe('Notifications: adding a channel', () => {
  const open = async () => {
    const user = userEvent.setup()
    await user.click(within(section()).getByRole('button', { name: /Add channel/ }))
    return { user, dialog: await screen.findByRole('dialog') }
  }

  it('starts as ntfy on ntfy.sh, and shows the fields for the type chosen', async () => {
    show()
    const { user, dialog } = await open()
    const d = within(dialog)
    expect(d.getByRole('radio', { name: /ntfy/ })).toBeChecked()
    expect(d.getByLabelText('Server')).toHaveValue('https://ntfy.sh')
    expect(d.getByLabelText('Topic')).toBeInTheDocument()
    expect(d.queryByLabelText('User key')).not.toBeInTheDocument()

    await user.click(d.getByRole('radio', { name: /Pushover/ }))
    expect(d.getByLabelText('Application token')).toBeInTheDocument()
    expect(d.getByLabelText('User key')).toBeInTheDocument()
    expect(d.queryByLabelText('Topic')).not.toBeInTheDocument()

    await user.click(d.getByRole('radio', { name: /Webhook/ }))
    expect(d.getByLabelText('Webhook address')).toHaveValue('')
    expect(d.getByLabelText(/Token/)).toBeInTheDocument()
    expect(d.queryByLabelText('Topic')).not.toBeInTheDocument()
  })

  it('hides tokens as they are typed', async () => {
    show()
    const { dialog } = await open()
    expect(within(dialog).getByLabelText(/Access token/)).toHaveAttribute('type', 'password')
  })

  it('sends the form and closes', async () => {
    const { fn, calls } = stubFetch({ 'POST /notifications/channels': () => ({ status: 201, body: { ...phone, name: 'My phone' } }), ...ok })
    vi.stubGlobal('fetch', fn)
    show()
    const { user, dialog } = await open()
    const d = within(dialog)
    await user.type(d.getByLabelText('Name'), 'My phone')
    await user.type(d.getByLabelText('Topic'), 'tv-x7k2')
    await user.type(d.getByLabelText(/Access token/), 'tk_secret')
    await user.click(d.getByRole('button', { name: 'Add channel' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'My phone', type: 'ntfy', url: 'https://ntfy.sh', topic: 'tv-x7k2', token: 'tk_secret' })
    expect(await screen.findByText('Added My phone')).toBeInTheDocument()
  })

  it('sends only the fields for the type: Pushover has no address or topic', async () => {
    const { fn, calls } = stubFetch({ 'POST /notifications/channels': () => ({ status: 201, body: pushover }), ...ok })
    vi.stubGlobal('fetch', fn)
    show()
    const { user, dialog } = await open()
    const d = within(dialog)
    await user.type(d.getByLabelText('Name'), 'Pushover')
    await user.click(d.getByRole('radio', { name: /Pushover/ }))
    await user.type(d.getByLabelText('Application token'), 'app')
    await user.type(d.getByLabelText('User key'), 'usr')
    await user.click(d.getByRole('button', { name: 'Add channel' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Pushover', type: 'pushover', token: 'app', user: 'usr' })
  })

  it("shows the server's reason and keeps the dialog open", async () => {
    vi.stubGlobal('fetch', stubFetch({ 'POST /notifications/channels': () => ({ status: 400, body: { error: 'channel "Phone": the topic must be 1 to 64 letters, digits, \'_\' or \'-\'' } }) }).fn)
    show()
    const { user, dialog } = await open()
    await user.type(within(dialog).getByLabelText('Name'), 'Phone')
    await user.click(within(dialog).getByRole('button', { name: 'Add channel' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('the topic must be')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('has a Send test that tests what is in the form, before anything is saved', async () => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => (release = resolve))
    const { fn, calls } = stubFetch({ 'POST /notifications/test': () => ({ body: { ok: true } }) })
    vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') await gate
      return fn(input, init)
    })
    show()
    const { user, dialog } = await open()
    const d = within(dialog)
    await user.type(d.getByLabelText('Name'), 'My phone')
    await user.type(d.getByLabelText('Topic'), 'tv-x7k2')

    await user.click(d.getByRole('button', { name: 'Send test' }))
    const busy = await d.findByRole('button', { name: 'Sending…' })
    expect(busy).toBeDisabled()
    release()
    expect(await screen.findByText('Test sent to My phone')).toBeInTheDocument()
    expect(await d.findByRole('button', { name: 'Send test' })).toBeEnabled()
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ channel: { name: 'My phone', type: 'ntfy', url: 'https://ntfy.sh', topic: 'tv-x7k2' } })
    expect(calls.filter((c) => c.url.endsWith('/notifications/channels'))).toEqual([]) // nothing was saved
  })

  it("tells you why a test didn't go through, and stays open", async () => {
    vi.stubGlobal('fetch', stubFetch({ 'POST /notifications/test': () => ({ status: 400, body: { error: 'give the channel a name' } }) }).fn)
    show()
    const { user, dialog } = await open()
    await user.click(within(dialog).getByRole('button', { name: 'Send test' }))
    expect(await screen.findByText('give the channel a name')).toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('cancels without sending anything', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    show()
    const { user, dialog } = await open()
    await user.click(within(dialog).getByRole('button', { name: /Cancel/ }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(calls).toEqual([])
  })
})

describe('Notifications: changing a channel', () => {
  const edit = async (state: State, name: string) => {
    show(state)
    const user = userEvent.setup()
    await user.click(within(section()).getByRole('button', { name: `Edit ${name}` }))
    return { user, dialog: await screen.findByRole('dialog') }
  }

  it('shows what is saved, but never a secret: a saved token is only said to be saved', async () => {
    const { dialog } = await edit(withChannels(makeState(), phone), 'Phone')
    const d = within(dialog)
    expect(d.getByRole('heading', { name: 'Edit Phone' })).toBeInTheDocument()
    expect(d.getByLabelText('Name')).toHaveValue('Phone')
    expect(d.getByLabelText('Server')).toHaveValue('https://ntfy.sh')
    expect(d.getByLabelText('Topic')).toHaveValue('antennarr')
    const token = d.getByLabelText(/Access token/)
    expect(token).toHaveValue('')
    expect(token).toHaveAttribute('placeholder', 'Saved. Leave empty to keep it.')
  })

  it('saves without sending the token, so the saved one is kept', async () => {
    const { fn, calls } = stubFetch({ 'PUT /notifications/channels/c1': () => ({ body: phone }), ...ok })
    vi.stubGlobal('fetch', fn)
    const { user, dialog } = await edit(withChannels(makeState(), phone), 'Phone')
    const topic = within(dialog).getByLabelText('Topic')
    await user.clear(topic)
    await user.type(topic, 'newtopic')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
    const body = calls.find((c) => c.method === 'PUT')?.body as Record<string, unknown>
    expect(body).toEqual({ name: 'Phone', type: 'ntfy', url: 'https://ntfy.sh', topic: 'newtopic' })
    expect(body).not.toHaveProperty('token')
    expect(await screen.findByText('Saved Phone')).toBeInTheDocument()
  })

  it('removes a saved token only when asked', async () => {
    const { fn, calls } = stubFetch({ 'PUT /notifications/channels/c1': () => ({ body: phone }), ...ok })
    vi.stubGlobal('fetch', fn)
    const { user, dialog } = await edit(withChannels(makeState(), phone), 'Phone')
    await user.click(within(dialog).getByRole('checkbox', { name: 'Remove the saved token' }))
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
    expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({ clearToken: true })
  })

  it('offers no way to remove a token that is not there', async () => {
    const { dialog } = await edit(withChannels(makeState(), discord), 'Discord')
    expect(within(dialog).queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it("starts a webhook's address empty, since it is never sent to the browser, and says where it goes", async () => {
    const { dialog } = await edit(withChannels(makeState(), discord), 'Discord')
    const address = within(dialog).getByLabelText('Webhook address')
    expect(address).toHaveValue('')
    expect(address).toHaveAttribute('placeholder', 'Saved (discord.com). Leave empty to keep it.')
  })

  it('tests the saved channel with the edits in the form', async () => {
    const { fn, calls } = stubFetch({ 'POST /notifications/test': () => ({ body: { ok: true } }) })
    vi.stubGlobal('fetch', fn)
    const { user, dialog } = await edit(withChannels(makeState(), phone), 'Phone')
    const topic = within(dialog).getByLabelText('Topic')
    await user.clear(topic)
    await user.type(topic, 'edited')
    await user.click(within(dialog).getByRole('button', { name: 'Send test' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ id: 'c1', channel: { name: 'Phone', type: 'ntfy', url: 'https://ntfy.sh', topic: 'edited' } })
  })

  it('drops the saved secrets when the type changes, since they belong to another service', async () => {
    const { fn, calls } = stubFetch({ 'PUT /notifications/channels/c1': () => ({ body: pushover }), ...ok })
    vi.stubGlobal('fetch', fn)
    const { user, dialog } = await edit(withChannels(makeState(), phone), 'Phone')
    await user.click(within(dialog).getByRole('radio', { name: /Pushover/ }))
    const token = within(dialog).getByLabelText('Application token')
    expect(token).toHaveAttribute('placeholder', '') // the ntfy token is not offered to Pushover
    await user.type(token, 'app')
    await user.type(within(dialog).getByLabelText('User key'), 'usr')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
    expect(calls.find((c) => c.method === 'PUT')?.body).toEqual({ name: 'Phone', type: 'pushover', token: 'app', user: 'usr' })
  })
})

describe('Notifications: removing a channel', () => {
  it('asks first, then removes it', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /notifications/channels/c1': () => ({ status: 204 }), ...ok })
    vi.stubGlobal('fetch', fn)
    show(withChannels(makeState(), phone))
    const user = userEvent.setup()
    await user.click(within(section()).getByRole('button', { name: 'Remove Phone' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Remove Phone?')).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    await user.click(within(dialog).getByRole('button', { name: /Remove/ }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/notifications/channels/c1'))).toBe(true))
    expect(await screen.findByText('Removed Phone')).toBeInTheDocument()
  })

  it('keeps it when you change your mind', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    show(withChannels(makeState(), phone))
    const user = userEvent.setup()
    await user.click(within(section()).getByRole('button', { name: 'Remove Phone' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Keep/ }))
    expect(calls).toEqual([])
  })
})
