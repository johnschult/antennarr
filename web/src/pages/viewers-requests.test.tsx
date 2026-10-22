import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { suggestName } from '@/hooks/use-allow-fields'
import type { AccessRequest } from '@/lib/types'
import { makeState, makeTwoDeviceState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { ViewersPage } from './viewers'

afterEach(() => vi.unstubAllGlobals())

const base = { device: 'antenna', first: '2026-10-22T12:00:00Z', last: '2026-10-22T12:05:00Z', count: 1 }
const pal: AccessRequest = { ...base, id: 'aaaaaaaaaaaa', label: 'pal@example.com', address: '100.64.0.30', login: 'pal@example.com', node: 'den.tail1.ts.net', count: 3 }
const kiosk: AccessRequest = { ...base, id: 'bbbbbbbbbbbb', label: 'kiosk.tail1.ts.net', address: '100.64.0.31', node: 'kiosk.tail1.ts.net' }
const bare: AccessRequest = { ...base, id: 'cccccccccccc', label: '10.8.0.44', address: '10.8.0.44' }

const asking = (requests: AccessRequest[], over = {}) => makeState({ requests, ...over })

describe('Requests', () => {
  it('has no section while nobody is asking', () => {
    renderWithApp(<ViewersPage />, makeState())
    expect(screen.queryByRole('heading', { name: /Asking to use your tuner/ })).not.toBeInTheDocument()
  })

  it('lists who asked, by who they are, with how often and when', () => {
    renderWithApp(<ViewersPage />, asking([pal, kiosk, bare]))
    const list = screen.getByRole('list', { name: 'Requests' })
    const item = (label: string) => within(list).getByText(label).closest('li') as HTMLElement
    expect(within(item('pal@example.com')).getByText(/den\.tail1\.ts\.net · asked 3 times/)).toBeInTheDocument()
    expect(within(item('kiosk.tail1.ts.net')).getByText(/100\.64\.0\.31 · asked once/)).toBeInTheDocument()
    expect(within(item('10.8.0.44')).getByText(/^asked once/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Allow pal@example.com' })).toBeEnabled()
  })

  it('names the tuner they asked for when there are several', () => {
    renderWithApp(<ViewersPage />, { ...makeTwoDeviceState(), requests: [{ ...bare, device: 'cable' }] })
    expect(screen.getByText(/asked once · .* · for .*[Cc]able/)).toBeInTheDocument()
  })

  it('does not name the tuner when there is only one', () => {
    renderWithApp(<ViewersPage />, asking([bare]))
    expect(within(screen.getByRole('list', { name: 'Requests' })).queryByText(/ for /)).not.toBeInTheDocument()
  })

  it('cannot be allowed or dismissed when settings cannot be saved', () => {
    renderWithApp(<ViewersPage />, asking([pal], { editable: false }))
    expect(screen.getByRole('button', { name: 'Allow pal@example.com' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Dismiss pal@example.com' })).toBeDisabled()
  })

  it('tells someone with no viewers that they can just try connecting', () => {
    renderWithApp(<ViewersPage />, makeState({ viewers: [] }))
    expect(screen.getByText(/ask them to add Yagi in their media server/)).toBeInTheDocument()
  })
})

describe('Allowing a request', () => {
  it('offers a name from their login, says how they will be recognized, and sends only a name and a limit', async () => {
    const { fn, calls } = stubFetch({ 'POST /requests/aaaaaaaaaaaa/allow': () => ({ status: 201, body: {} }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, asking([pal]))
    await user.click(screen.getByRole('button', { name: 'Allow pal@example.com' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Allow pal@example.com')).toBeInTheDocument()
    expect(within(dialog).getByText(/recognized by their Tailscale login \(pal@example\.com\), from any of their devices/)).toBeInTheDocument()
    const name = within(dialog).getByLabelText('Name') as HTMLInputElement
    expect(name.value).toBe('Pal')
    await user.clear(name)
    await user.type(name, 'Pal Friend')
    await user.clear(within(dialog).getByLabelText('Tuners at once'))
    await user.type(within(dialog).getByLabelText('Tuners at once'), '2')
    await user.click(within(dialog).getByRole('button', { name: /^Allow$/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Pal Friend', limit: 2 }))
  })

  it('says how a device or a bare address will be recognized', async () => {
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, asking([kiosk, bare]))
    await user.click(screen.getByRole('button', { name: 'Allow kiosk.tail1.ts.net' }))
    expect(await screen.findByText(/recognized by that device \(kiosk\.tail1\.ts\.net\)/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Cancel/ }))
    await user.click(screen.getByRole('button', { name: 'Allow 10.8.0.44' }))
    expect(await screen.findByText(/recognized by their address \(10\.8\.0\.44\)/)).toBeInTheDocument()
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('') // nothing to suggest from an address
  })

  it('checks the form before asking, and shows what the server says', async () => {
    const { fn, calls } = stubFetch({ 'POST /requests/cccccccccccc/allow': () => ({ status: 409, body: { error: 'viewer "Dad" already exists' } }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, asking([bare]))
    await user.click(screen.getByRole('button', { name: 'Allow 10.8.0.44' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: /^Allow$/ }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent('Give the viewer a name.')
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(0)
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.click(within(dialog).getByRole('button', { name: /^Allow$/ }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('viewer "Dad" already exists')
  })

  it('sends a limit for each tuner that has one when there are several', async () => {
    const { fn, calls } = stubFetch({ 'POST /requests/cccccccccccc/allow': () => ({ status: 201, body: {} }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, { ...makeTwoDeviceState(), requests: [bare] })
    await user.click(screen.getByRole('button', { name: 'Allow 10.8.0.44' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Dad')
    await user.type(within(dialog).getByLabelText(/Tuners on Cable/), '3')
    await user.click(within(dialog).getByRole('button', { name: /^Allow$/ }))
    await waitFor(() => expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Dad', limit: 1, limits: { cable: 3 } }))
  })

  it('dismisses with one press', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /requests/aaaaaaaaaaaa': () => ({ status: 204 }), 'GET /state': () => ({ body: makeState() }) })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    renderWithApp(<ViewersPage />, asking([pal]))
    await user.click(screen.getByRole('button', { name: 'Dismiss pal@example.com' }))
    await waitFor(() => expect(calls.find((c) => c.method === 'DELETE')?.url).toMatch(/\/requests\/aaaaaaaaaaaa$/))
  })
})

describe('suggestName', () => {
  it('offers a name from a login or a device, and none from an address', () => {
    expect(suggestName(pal)).toBe('Pal')
    expect(suggestName({ ...pal, login: 'mary.jane_smith@example.com' })).toBe('Mary Jane Smith')
    expect(suggestName(kiosk)).toBe('Kiosk')
    expect(suggestName({ ...kiosk, node: 'living-room-pc.tail1.ts.net' })).toBe('Living Room Pc')
    expect(suggestName(bare)).toBe('')
  })
})
