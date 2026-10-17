import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FakeEventSource } from '@/test/fake-event-source'
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

describe('useApp', () => {
  it('refuses to be used outside the provider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Probe />)).toThrow('useApp must be used inside AppStateProvider')
    spy.mockRestore()
  })
})
