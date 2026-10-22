import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Invite, InviteCreated } from '@/lib/types'
import { john, makeState, makeTwoDeviceState, mom } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { ViewersPage } from './viewers'

afterEach(() => vi.unstubAllGlobals())

const inviting = (over: Partial<ReturnType<typeof makeState>> = {}) => {
  const s = makeState(over)
  s.settings.plex = { enabled: false, admins: [], enrollment: true } // inviting does not need Plex sign-in
  return s
}

const waiting: Invite = { id: 'aaaaaaaaaaaa', name: 'Dad', limit: 1, created: '2026-10-22T12:00:00Z', expires: '2999-01-01T00:00:00Z', used: false, expired: false }
const joined: Invite = { ...waiting, id: 'bbbbbbbbbbbb', name: 'Mom2', used: true, viewer: 'Mom2' }
const expired: Invite = { ...waiting, id: 'cccccccccccc', name: 'Old', expired: true, expires: '2020-01-01T00:00:00Z' }

describe('Inviting a viewer: the button', () => {
  it('is there when the invite switch is on, whether or not Plex sign-in is', () => {
    renderWithApp(<ViewersPage />, inviting())
    expect(screen.getByRole('button', { name: /Invite with Plex/ })).toBeEnabled()
  })

  it('is not there at all when the invite switch is off, and nothing is said about it', () => {
    renderWithApp(<ViewersPage />, makeState())
    expect(screen.queryByRole('button', { name: /Invite/ })).not.toBeInTheDocument()
    expect(screen.queryByText(/invite link/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Add viewer/ })).toBeEnabled()
  })

  it('is not there when only Plex sign-in is on: that is another thing', () => {
    const s = makeState()
    s.settings.plex = { enabled: true, admins: [], enrollment: false }
    renderWithApp(<ViewersPage />, s)
    expect(screen.queryByRole('button', { name: /Invite/ })).not.toBeInTheDocument()
  })

  it('is off when settings cannot be saved', () => {
    renderWithApp(<ViewersPage />, inviting({ editable: false }))
    expect(screen.getByRole('button', { name: /Invite with Plex/ })).toBeDisabled()
  })
})

describe('Inviting a viewer: the dialog', () => {
  const made: InviteCreated = { ...waiting, name: 'Dad', path: '/invite/TOKENTOKENTOKENTOKENTOKENTOKENTOKENTOKEN00', url: 'http://localhost:9244/invite/TOKENTOKENTOKENTOKENTOKENTOKENTOKENTOKEN00' }

  it('asks for a name and a limit, makes the invite, and shows the link', async () => {
    const { fn, calls } = stubFetch({
      'POST /invites': () => ({ status: 201, body: made }),
      'GET /network': () => ({ body: { addresses: [{ ip: '192.168.1.5', kind: 'lan', interface: 'en0' }, { ip: '100.64.0.5', kind: 'tailscale', interface: 'utun3' }] } }),
      'GET /state': () => ({ body: inviting() }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting())

    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), '  Dad ')
    await user.clear(within(dialog).getByLabelText('Tuners at once'))
    await user.type(within(dialog).getByLabelText('Tuners at once'), '2')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))

    expect(await within(dialog).findByText('Send this link to Dad')).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Dad', limit: 2 })
    const link = within(dialog).getByLabelText('Invite link') as HTMLInputElement
    expect(link.value).toMatch(/\/invite\/TOKENTOKENTOKENTOKENTOKENTOKENTOKENTOKEN00$/)
    expect(within(dialog).getByText(/shown only this once/)).toBeInTheDocument()
  })

  it('offers the addresses the machine can be reached at, and uses the one chosen', async () => {
    const { fn } = stubFetch({
      'POST /invites': () => ({ status: 201, body: made }),
      'GET /network': () => ({ body: { addresses: [{ ip: '192.168.1.5', kind: 'lan', interface: 'en0' }, { ip: '100.64.0.5', kind: 'tailscale', interface: 'utun3' }] } }),
      'GET /state': () => ({ body: inviting() }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting())
    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))

    const address = (await within(dialog).findByLabelText('Address they will open it at')) as HTMLSelectElement
    await waitFor(() => expect(Array.from(address.options).map((o) => o.textContent)).toContain('Tailscale: 100.64.0.5'))
    // The page is open on localhost in a test, which no viewer can reach, so a real address is chosen first.
    expect((within(dialog).getByLabelText('Invite link') as HTMLInputElement).value.startsWith('http://100.64.0.5')).toBe(true)
    await user.selectOptions(address, 'Network: 192.168.1.5')
    expect((within(dialog).getByLabelText('Invite link') as HTMLInputElement).value).toMatch(/^http:\/\/192\.168\.1\.5(:\d+)?\/invite\//)
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
  })

  it('warns when the only address is one that works only on this computer', async () => {
    const { fn } = stubFetch({
      'POST /invites': () => ({ status: 201, body: made }),
      'GET /network': () => ({ body: { addresses: [] } }),
      'GET /state': () => ({ body: inviting() }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting())
    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('only works on this computer')
  })

  it('copies the link', async () => {
    const { fn } = stubFetch({
      'POST /invites': () => ({ status: 201, body: made }),
      'GET /network': () => ({ body: { addresses: [] } }),
      'GET /state': () => ({ body: inviting() }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting())
    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    await user.click(await within(dialog).findByRole('button', { name: 'Copy the invite link' }))
    // userEvent stands in for the clipboard, so what was copied can be read back.
    expect(await navigator.clipboard.readText()).toMatch(/\/invite\/TOKENTOKEN.*00$/)
    expect(await within(dialog).findByText('Copied')).toBeInTheDocument()
  })

  it('checks the form before asking, and shows what the server says', async () => {
    const { fn, calls } = stubFetch({ 'POST /invites': () => ({ status: 409, body: { error: 'a viewer named "Dad" already exists' } }), 'GET /state': () => ({ body: inviting() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting())
    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Give the viewer a name.')
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(0)

    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.clear(within(dialog).getByLabelText('Tuners at once'))
    await user.type(within(dialog).getByLabelText('Tuners at once'), '-1')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('must be 0 or more')

    await user.clear(within(dialog).getByLabelText('Tuners at once'))
    await user.type(within(dialog).getByLabelText('Tuners at once'), '1')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('a viewer named "Dad" already exists')
  })

  it('sends a limit for each device that has one when there are several', async () => {
    const { fn, calls } = stubFetch({ 'POST /invites': () => ({ status: 201, body: made }), 'GET /network': () => ({ body: { addresses: [] } }), 'GET /state': () => ({ body: inviting() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    const s = makeTwoDeviceState()
    s.settings.plex = { enabled: false, admins: [], enrollment: true } // inviting does not need Plex sign-in
    renderWithApp(<ViewersPage />, s)
    await user.click(screen.getByRole('button', { name: /Invite with Plex/ }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.type(within(dialog).getByLabelText(/Tuners on Cable/), '3')
    await user.click(within(dialog).getByRole('button', { name: /Make the link/ }))
    await within(dialog).findByText('Send this link to Dad')
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Dad', limit: 1, limits: { cable: 3 } })
  })
})

describe('Invites: the list', () => {
  it('shows each invite by where it stands, and nothing when there are none', () => {
    renderWithApp(<ViewersPage />, inviting({ invites: [waiting, joined, expired] }))
    const list = screen.getByRole('list', { name: 'Invites' })
    const item = (name: string) => within(list).getByText(name).closest('li') as HTMLElement
    expect(within(item('Dad')).getByText('Waiting')).toBeInTheDocument()
    expect(within(item('Dad')).getByText(/Not used yet\. It stops working in/)).toBeInTheDocument()
    expect(within(item('Mom2')).getByText('Joined')).toBeInTheDocument()
    expect(within(item('Mom2')).getByText(/update their address/)).toBeInTheDocument()
    expect(within(item('Old')).getByText('Expired')).toBeInTheDocument()
  })

  it('has no Invites section while there are none', () => {
    renderWithApp(<ViewersPage />, inviting())
    expect(screen.queryByRole('heading', { name: 'Invites' })).not.toBeInTheDocument()
  })

  it('revokes one after asking, and says so', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /invites/aaaaaaaaaaaa': () => ({ status: 204 }), 'GET /state': () => ({ body: inviting() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting({ invites: [waiting] }))
    await user.click(screen.getByRole('button', { name: 'Revoke the invite for Dad' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Revoke the invite for Dad\?/)).toBeInTheDocument()
    expect(calls.filter((c) => c.method === 'DELETE')).toHaveLength(0) // asking is not doing
    await user.click(within(dialog).getByRole('button', { name: /Revoke/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'DELETE')?.url).toMatch(/\/invites\/aaaaaaaaaaaa$/))
  })

  it('says what revoking a used invite does and does not do', async () => {
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting({ invites: [joined] }))
    await user.click(screen.getByRole('button', { name: 'Revoke the invite for Mom2' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Mom2 stays a viewer/)).toBeInTheDocument()
  })

  it('cannot revoke when settings cannot be saved', () => {
    renderWithApp(<ViewersPage />, inviting({ invites: [waiting], editable: false }))
    expect(screen.getByRole('button', { name: 'Revoke the invite for Dad' })).toBeDisabled()
  })
})

describe('Viewers who joined with Plex', () => {
  it('say so, with the account and the server they came from', () => {
    const enrolled = { ...mom, name: 'Dad', plex: { accountId: 9, accountName: 'dad', serverId: 'mid', serverName: "Dad's Plex" } }
    renderWithApp(<ViewersPage />, inviting({ viewers: [john, enrolled] }))
    const row = screen.getByRole('row', { name: /Dad/ })
    expect(within(row).getByText("Plex: dad · Dad's Plex")).toBeInTheDocument()
    expect(within(screen.getByRole('row', { name: /John/ })).queryByText(/^Plex:/)).not.toBeInTheDocument()
  })

  it('and the edit dialog explains that their addresses are replaced when they refresh', async () => {
    const enrolled = { ...mom, name: 'Dad', plex: { accountId: 9, accountName: 'dad', serverId: 'mid', serverName: "Dad's Plex" } }
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, inviting({ viewers: [john, enrolled] }))
    await user.click(screen.getByRole('button', { name: 'Edit Dad' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Joined through an invite/)).toHaveTextContent("Dad's Plex")
    expect(within(dialog).getByText(/replaced with that server's current ones/)).toBeInTheDocument()
  })
})
