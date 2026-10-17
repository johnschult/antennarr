import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { API } from '@/lib/api'
import { FakeEventSource } from '@/test/fake-event-source'
import { makeState } from '@/test/fixtures'
import { stubFetch } from '@/test/render'
import { useLiveState } from './use-live-state'

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

beforeEach(() => {
  FakeEventSource.install()
  vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ body: makeState() }) }).fn)
})
afterEach(() => vi.unstubAllGlobals())

describe('useLiveState', () => {
  it('connects to the event stream and starts out not connected', async () => {
    const { result } = renderHook(() => useLiveState(), { wrapper })
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.last().url).toBe(`${API}/events`)
    expect(result.current.connected).toBe(false)
    await waitFor(() => expect(result.current.state).toBeDefined()) // from the initial fetch
  })

  it('reports connected once the stream opens, and not when it drops', async () => {
    const { result } = renderHook(() => useLiveState(), { wrapper })
    act(() => FakeEventSource.last().open())
    expect(result.current.connected).toBe(true)
    act(() => FakeEventSource.last().fail())
    expect(result.current.connected).toBe(false)
  })

  it('replaces the state with each event pushed by the server', async () => {
    const { result } = renderHook(() => useLiveState(), { wrapper })
    await waitFor(() => expect(result.current.state?.summary.inUse).toBe(2))

    const next = makeState()
    next.summary = { ...next.summary, inUse: 3 }
    act(() => FakeEventSource.last().emit('state', next))

    await waitFor(() => expect(result.current.state?.summary.inUse).toBe(3))
    expect(result.current.connected).toBe(true) // receiving events means the stream is up
  })

  it('shows the pushed state even if the initial fetch failed', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ status: 500, body: { error: 'nope' } }) }).fn)
    const { result } = renderHook(() => useLiveState(), { wrapper })
    await waitFor(() => expect(result.current.error).toBeTruthy())

    act(() => FakeEventSource.last().emit('state', makeState()))
    await waitFor(() => expect(result.current.state?.tuner.id).toBe('10ABCC87'))
  })

  it('closes the stream when unmounted', () => {
    const { unmount } = renderHook(() => useLiveState(), { wrapper })
    const es = FakeEventSource.last()
    expect(es.closed).toBe(false)
    unmount()
    expect(es.closed).toBe(true)
  })
})
