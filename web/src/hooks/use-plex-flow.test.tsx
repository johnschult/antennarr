import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fakeWindow, stubOpen, type FakeWindow } from '@/test/fake-window'
import { stubFetch } from '@/test/render'
import { POLL_EVERY, usePlexFlow } from './use-plex-flow'

type Route = Parameters<typeof stubFetch>[0]

beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const start = { pinId: 11, authUrl: 'https://app.plex.tv/auth#?code=abc' }

function install(routes: Route) {
  const { fn, calls } = stubFetch(routes)
  vi.stubGlobal('fetch', fn)
  return calls
}

/** Run a hook, click "start", and let the start request finish. */
async function begin(opts: Parameters<typeof usePlexFlow>[0] = {}, win: FakeWindow | null = fakeWindow()) {
  stubOpen(win)
  const r = renderHook(() => usePlexFlow(opts))
  await act(async () => {
    void r.result.current.start()
    await vi.advanceTimersByTimeAsync(0)
  })
  return { ...r, win }
}

const tick = (ms = POLL_EVERY) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)))

describe('usePlexFlow', () => {
  it('opens a blank window at once, cut off from the page, then points it at Plex', async () => {
    install({ 'POST /auth/plex/start': () => ({ body: start }) })
    const { result, win } = await begin()
    expect(window.open).toHaveBeenCalledWith('', '_blank', expect.stringContaining('popup'))
    expect(win!.opener).toBeNull() // the Plex page must not be able to reach back into this one
    expect(win!.location.href).toBe(start.authUrl)
    expect(result.current.flow).toEqual({ phase: 'waiting', authUrl: start.authUrl })
  })

  it('asks the server every two seconds until the person has finished', async () => {
    const calls = install({
      'POST /auth/plex/start': () => ({ body: start }),
      'POST /auth/plex/poll': (() => {
        let n = 0
        return () => ({ body: ++n < 3 ? { status: 'pending' } : { status: 'ok', user: { id: 7, name: 'john' } } })
      })(),
    })
    const onDone = vi.fn()
    const { result, win } = await begin({ onDone })

    expect(calls.filter((c) => c.url.endsWith('/poll'))).toHaveLength(0) // not before the first interval
    await tick()
    await tick()
    expect(result.current.flow.phase).toBe('waiting')
    expect(onDone).not.toHaveBeenCalled()
    await tick()
    expect(result.current.flow).toEqual({ phase: 'done', name: 'john' })
    expect(onDone).toHaveBeenCalledTimes(1)
    expect(win!.close).toHaveBeenCalled()
    expect(calls.filter((c) => c.url.endsWith('/poll')).every((c) => (c.body as { pinId: number }).pinId === 11)).toBe(true)

    await tick(10_000) // and it stops asking
    expect(calls.filter((c) => c.url.endsWith('/poll'))).toHaveLength(3)
  })

  it('reports an account that is not allowed', async () => {
    install({
      'POST /auth/plex/start': () => ({ body: start }),
      'POST /auth/plex/poll': () => ({ body: { status: 'denied', user: { name: 'stranger' } } }),
    })
    const onDone = vi.fn()
    const { result, win } = await begin({ onDone })
    await tick()
    expect(result.current.flow).toEqual({ phase: 'denied', name: 'stranger' })
    expect(onDone).not.toHaveBeenCalled()
    expect(win!.close).toHaveBeenCalled()
  })

  it('reports a sign-in that ran out', async () => {
    install({ 'POST /auth/plex/start': () => ({ body: start }), 'POST /auth/plex/poll': () => ({ body: { status: 'expired' } }) })
    const { result } = await begin()
    await tick()
    expect(result.current.flow.phase).toBe('expired')
  })

  it('shows why it could not start, and closes the window it opened', async () => {
    install({ 'POST /auth/plex/start': () => ({ status: 502, body: { error: "couldn't start a sign-in with plex.tv: boom" } }) })
    const { result, win } = await begin()
    expect(result.current.flow).toEqual({ phase: 'error', message: "couldn't start a sign-in with plex.tv: boom" })
    expect(win!.close).toHaveBeenCalled()
  })

  it('stops and says so when polling fails', async () => {
    const calls = install({
      'POST /auth/plex/start': () => ({ body: start }),
      'POST /auth/plex/poll': () => ({ status: 502, body: { error: "couldn't ask plex.tv: down" } }),
    })
    const { result } = await begin()
    await tick()
    expect(result.current.flow).toEqual({ phase: 'error', message: "couldn't ask plex.tv: down" })
    await tick(10_000)
    expect(calls.filter((c) => c.url.endsWith('/poll'))).toHaveLength(1)
  })

  it('still works when a pop-up blocker stops the window, by offering the link', async () => {
    install({ 'POST /auth/plex/start': () => ({ body: start }), 'POST /auth/plex/poll': () => ({ body: { status: 'pending' } }) })
    const { result } = await begin({}, null)
    expect(result.current.flow).toEqual({ phase: 'waiting', authUrl: start.authUrl })
  })

  it('can be cancelled, which stops the polling and closes the window', async () => {
    const calls = install({ 'POST /auth/plex/start': () => ({ body: start }), 'POST /auth/plex/poll': () => ({ body: { status: 'pending' } }) })
    const { result, win } = await begin()
    await tick()
    act(() => result.current.cancel())
    expect(result.current.flow.phase).toBe('idle')
    expect(win!.close).toHaveBeenCalled()
    const before = calls.length
    await tick(10_000)
    expect(calls).toHaveLength(before)
  })

  it('abandons the sign-in when the page goes away', async () => {
    const calls = install({ 'POST /auth/plex/start': () => ({ body: start }), 'POST /auth/plex/poll': () => ({ body: { status: 'pending' } }) })
    const { unmount, win } = await begin()
    unmount()
    expect(win!.close).toHaveBeenCalled()
    await tick(10_000)
    expect(calls.filter((c) => c.url.endsWith('/poll'))).toHaveLength(0)
  })

  it('starting again drops the first sign-in', async () => {
    let n = 0
    const calls = install({
      'POST /auth/plex/start': () => ({ body: { pinId: 100 + ++n, authUrl: `https://app.plex.tv/auth#?code=${n}` } }),
      'POST /auth/plex/poll': () => ({ body: { status: 'pending' } }),
    })
    const { result } = await begin()
    await act(async () => {
      void result.current.start()
      await vi.advanceTimersByTimeAsync(0)
    })
    await tick()
    const polled = calls.filter((c) => c.url.endsWith('/poll')).map((c) => (c.body as { pinId: number }).pinId)
    expect(polled).toEqual([102]) // only the newer sign-in is being asked about
  })

  it('adds an account through the add endpoints, and reports the account', async () => {
    const calls = install({
      'POST /auth/plex/add/start': () => ({ body: start }),
      'POST /auth/plex/add/poll': () => ({ body: { status: 'ok', account: { id: 42, name: 'steve' } } }),
    })
    const { result } = await begin({ add: true })
    await tick()
    expect(result.current.flow).toEqual({ phase: 'done', name: 'steve' })
    expect(calls.map((c) => c.url.replace(/^.*\/auth/, ''))).toEqual(['/plex/add/start', '/plex/add/poll'])
  })
})
