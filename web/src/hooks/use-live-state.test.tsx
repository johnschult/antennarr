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

  describe('when the browser gives up on the stream', () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] }))
    afterEach(() => vi.useRealTimers())

    it('opens a new one, slower each time, and goes back to quick retries once it works', async () => {
      const { result } = renderHook(() => useLiveState(), { wrapper })
      expect(FakeEventSource.instances).toHaveLength(1)

      act(() => FakeEventSource.last().fail(true))
      expect(result.current.connected).toBe(false)
      act(() => void vi.advanceTimersByTime(2_999))
      expect(FakeEventSource.instances).toHaveLength(1) // not yet
      act(() => void vi.advanceTimersByTime(1))
      expect(FakeEventSource.instances).toHaveLength(2)

      act(() => FakeEventSource.last().fail(true)) // still refused: wait twice as long
      act(() => void vi.advanceTimersByTime(5_999))
      expect(FakeEventSource.instances).toHaveLength(2)
      act(() => void vi.advanceTimersByTime(1))
      expect(FakeEventSource.instances).toHaveLength(3)

      act(() => FakeEventSource.last().open())
      expect(result.current.connected).toBe(true)
      act(() => FakeEventSource.last().fail(true)) // after a success the wait starts over
      act(() => void vi.advanceTimersByTime(3_000))
      expect(FakeEventSource.instances).toHaveLength(4)
    })

    it('never waits more than 15 seconds', () => {
      renderHook(() => useLiveState(), { wrapper })
      for (let i = 0; i < 8; i++) {
        act(() => FakeEventSource.last().fail(true))
        act(() => void vi.advanceTimersByTime(15_000))
      }
      const n = FakeEventSource.instances.length
      act(() => FakeEventSource.last().fail(true))
      act(() => void vi.advanceTimersByTime(15_000))
      expect(FakeEventSource.instances).toHaveLength(n + 1)
    })

    it("leaves the browser's own retries alone when it has not given up", () => {
      renderHook(() => useLiveState(), { wrapper })
      act(() => FakeEventSource.last().fail()) // a lost connection: the browser reconnects itself
      act(() => void vi.advanceTimersByTime(60_000))
      expect(FakeEventSource.instances).toHaveLength(1)
    })

    it('stops trying when the page goes away', () => {
      const { unmount } = renderHook(() => useLiveState(), { wrapper })
      act(() => FakeEventSource.last().fail(true))
      unmount()
      act(() => void vi.advanceTimersByTime(60_000))
      expect(FakeEventSource.instances).toHaveLength(1)
    })
  })

  it('replaces the state with each event pushed by the server', async () => {
    const { result } = renderHook(() => useLiveState(), { wrapper })
    await waitFor(() => expect(result.current.state?.devices[0].summary.inUse).toBe(2))

    const next = makeState()
    next.devices[0].summary = { ...next.devices[0].summary, inUse: 3 }
    act(() => FakeEventSource.last().emit('state', next))

    await waitFor(() => expect(result.current.state?.devices[0].summary.inUse).toBe(3))
    expect(result.current.connected).toBe(true) // receiving events means the stream is up
  })

  it('shows the pushed state even if the initial fetch failed', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /state': () => ({ status: 500, body: { error: 'nope' } }) }).fn)
    const { result } = renderHook(() => useLiveState(), { wrapper })
    await waitFor(() => expect(result.current.error).toBeTruthy())

    act(() => FakeEventSource.last().emit('state', makeState()))
    await waitFor(() => expect(result.current.state?.devices[0].tuner.id).toBe('10ABCC87'))
  })

  it('closes the stream when unmounted', () => {
    const { unmount } = renderHook(() => useLiveState(), { wrapper })
    const es = FakeEventSource.last()
    expect(es.closed).toBe(false)
    unmount()
    expect(es.closed).toBe(true)
  })
})
