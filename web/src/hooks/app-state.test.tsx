import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FakeEventSource } from '@/test/fake-event-source'
import { fakeWindow, stubOpen } from '@/test/fake-window'
import { makeState } from '@/test/fixtures'
import { stubFetch } from '@/test/render'
import { useApp } from './app-context'
import { AppStateProvider } from './app-state'

function Probe() {
  const { state, connected } = useApp()
  return (
    <p>
      {state.devices[0].tuner.friendlyName} {connected ? 'live' : 'polling'}
    </p>
  )
}

const mount = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <AppStateProvider>
        <Probe />
      </AppStateProvider>
    </QueryClientProvider>,
  )

beforeEach(() => FakeEventSource.install())
afterEach(() => vi.unstubAllGlobals())

describe('AppStateProvider', () => {
  it('holds the page back until the state arrives, then renders it', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ body: makeState() }) }).fn)
    mount()
    expect(screen.getByText('Connecting to Antennarr…')).toBeInTheDocument()
    expect(await screen.findByText('Steve FLEX 4K polling')).toBeInTheDocument()
    act(() => FakeEventSource.last().open())
    expect(await screen.findByText('Steve FLEX 4K live')).toBeInTheDocument()
  })

  it('explains a 403 as "this address is not an admin"', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ status: 403, body: { error: 'admin only' } }) }).fn)
    mount()
    expect(await screen.findByText("This address can't open the admin page")).toBeInTheDocument()
    expect(screen.getByText(/add this computer's address to/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Try again' })).not.toBeInTheDocument()
  })

  it('shows other failures with the reason and a way to retry', async () => {
    vi.stubGlobal('fetch', async () => {
      throw new TypeError('offline')
    })
    mount()
    expect(await screen.findByText("Can't reach Antennarr")).toBeInTheDocument()
    expect(screen.getByText('Could not reach Antennarr')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
  })

  it('renders from a pushed event even when the first fetch fails', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ status: 500 }) }).fn)
    mount()
    await waitFor(() => expect(screen.getByText("Can't reach Antennarr")).toBeInTheDocument())
    act(() => FakeEventSource.last().emit('state', makeState()))
    expect(await screen.findByText(/Steve FLEX 4K/)).toBeInTheDocument()
  })
})

describe('signing in with Plex', () => {
  const forbidden = { status: 403, body: { error: 'admin only' } }
  const authOn = { 'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn: null, admin: false } }) }

  it('offers the button only when Plex sign-in is on', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => forbidden, 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: false } }) }).fn)
    mount()
    await screen.findByText("This address can't open the admin page")
    await waitFor(() => expect(screen.getByText(/Open it from the machine running Antennarr/)).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /Sign in with/ })).not.toBeInTheDocument()
  })

  it('signs in, then loads the page it was refused', async () => {
    let allowed = false
    const { fn, calls } = stubFetch({
      'GET /state': () => (allowed ? { body: makeState() } : forbidden),
      'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn: allowed ? { id: 7, name: 'john' } : null, admin: allowed } }),
      'POST /auth/plex/start': () => ({ body: { pinId: 5, authUrl: 'https://app.plex.tv/auth#?code=x' } }),
      'POST /auth/plex/poll': () => {
        allowed = true
        return { body: { status: 'ok', user: { id: 7, name: 'john' } } }
      },
    })
    vi.stubGlobal('fetch', fn)
    const win = fakeWindow()
    stubOpen(win)
    mount()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /Sign in with/ }))
    expect(win.location.href).toBe('https://app.plex.tv/auth#?code=x')
    expect(await screen.findByText('Steve FLEX 4K polling', {}, { timeout: 6000 })).toBeInTheDocument()
    expect(calls.some((c) => c.url.endsWith('/auth/plex/poll'))).toBe(true)
  }, 15000)

  it('says so when the account is not allowed', async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch({
        'GET /state': () => forbidden,
        ...authOn,
        'POST /auth/plex/start': () => ({ body: { pinId: 5, authUrl: 'https://app.plex.tv/auth' } }),
        'POST /auth/plex/poll': () => ({ body: { status: 'denied', user: { name: 'stranger' } } }),
      }).fn,
    )
    stubOpen(fakeWindow())
    mount()
    await userEvent.setup().click(await screen.findByRole('button', { name: /Sign in with/ }))
    expect(await screen.findByText(/“stranger” isn't allowed/, {}, { timeout: 6000 })).toBeInTheDocument()
  }, 15000)
})

describe('useApp', () => {
  it('refuses to be used outside the provider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Probe />)).toThrow('useApp must be used inside AppStateProvider')
    spy.mockRestore()
  })
})
