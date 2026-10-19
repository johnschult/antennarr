import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, addViewer, getAuthStatus, pollPlex, removePlexAdmin, signOut, startPlex, checkFirmware, deleteViewer, getState, patchDevice, patchSettings, replaceViewer } from './api'
import { stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

describe('api client', () => {
  it('sends JSON bodies with a JSON content type, and none on bodyless calls', async () => {
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      return new Response('{}', { status: 200 })
    })
    await addViewer({ name: 'X', addresses: ['1.2.3.4'], limit: 1 })
    await getState()
    expect(calls[0].init?.method).toBe('POST')
    expect(calls[0].init?.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(calls[0].init?.body).toBe('{"name":"X","addresses":["1.2.3.4"],"limit":1}')
    expect(calls[1].init?.headers).toBeUndefined()
    expect(calls[1].init?.body).toBeUndefined()
  })

  it('escapes viewer names in the path', async () => {
    const { fn, calls } = stubFetch({
      'PUT /viewers/Mom%20%26%20Dad': () => ({ body: {} }),
      'DELETE /viewers/Mom%20%26%20Dad': () => ({ status: 204 }),
    })
    vi.stubGlobal('fetch', fn)
    await replaceViewer('Mom & Dad', { name: 'Mom & Dad', addresses: ['1.2.3.4'], limit: 1 })
    await expect(deleteViewer('Mom & Dad')).resolves.toBeUndefined() // 204: no body to parse
    expect(calls.map((c) => `${c.method} ${c.url.replace(/^.*api/, '')}`)).toEqual([
      'PUT /viewers/Mom%20%26%20Dad',
      'DELETE /viewers/Mom%20%26%20Dad',
    ])
  })

  it("turns the server's error JSON into an ApiError with its message", async () => {
    const { fn } = stubFetch({ 'PATCH /settings': () => ({ status: 400, body: { error: 'ownerReserve cannot be negative' } }) })
    vi.stubGlobal('fetch', fn)
    const err = await patchSettings({ ownerReserve: -1 }).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(400)
    expect(err.message).toBe('ownerReserve cannot be negative')
  })

  it('copes with error replies that are not JSON', async () => {
    vi.stubGlobal('fetch', async () => new Response('<html>bad gateway</html>', { status: 502 }))
    const err = await getState().catch((e) => e)
    expect(err).toMatchObject({ name: 'ApiError', status: 502, message: 'Request failed (502)' })
  })

  it('reports an unreachable server as status 0', async () => {
    vi.stubGlobal('fetch', async () => {
      throw new TypeError('Failed to fetch')
    })
    const err = await getState().catch((e) => e)
    expect(err).toMatchObject({ status: 0, message: 'Could not reach Yagi' })
  })

  it('patches a device by name, escaping it', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/my%20box': () => ({ body: {} }) })
    vi.stubGlobal('fetch', fn)
    await patchDevice('my box', { ownerReserve: 0, sharedCap: 2 })
    expect(calls).toEqual([{ method: 'PATCH', url: expect.stringContaining('/devices/my%20box'), body: { ownerReserve: 0, sharedCap: 2 } }])
  })

  it('asks the server to read the changelog now', async () => {
    const { fn, calls } = stubFetch({ 'POST /firmware/check': () => ({ body: { checkedAt: '2026-10-17T10:00:00Z' } }) })
    vi.stubGlobal('fetch', fn)
    await expect(checkFirmware()).resolves.toEqual({ checkedAt: '2026-10-17T10:00:00Z' })
    expect(calls).toEqual([{ method: 'POST', url: expect.stringContaining('/firmware/check'), body: undefined }])
  })

  it('passes a rename through as a label', async () => {
    const { fn, calls } = stubFetch({ 'PATCH /devices/antenna': () => ({ body: {} }) })
    vi.stubGlobal('fetch', fn)
    await patchDevice('antenna', { label: 'Antenna (Steve)' })
    expect(calls[0].body).toEqual({ label: 'Antenna (Steve)' })
  })
})

describe('sign-in calls', () => {
  it('use the public /auth routes, and the add routes when adding an account', async () => {
    const { fn, calls } = stubFetch({
      'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn: null, admin: false } }),
      'POST /auth/plex/start': () => ({ body: { pinId: 1, authUrl: 'u' } }),
      'POST /auth/plex/add/start': () => ({ body: { pinId: 2, authUrl: 'u' } }),
      'POST /auth/plex/poll': () => ({ body: { status: 'pending' } }),
      'POST /auth/plex/add/poll': () => ({ body: { status: 'pending' } }),
      'POST /auth/logout': () => ({ body: {} }),
      'DELETE /plex/admins/42': () => ({ status: 204 }),
    })
    vi.stubGlobal('fetch', fn)
    expect((await getAuthStatus()).plexEnabled).toBe(true)
    await startPlex()
    await startPlex(true)
    await pollPlex(1)
    await pollPlex(2, true)
    await signOut()
    await removePlexAdmin(42)
    expect(calls.map((c) => `${c.method} ${c.url.replace(/^.*?(?=\/auth\/|\/api\/)/, '')}`)).toEqual([
      'GET /auth/status',
      'POST /auth/plex/start',
      'POST /auth/plex/add/start',
      'POST /auth/plex/poll',
      'POST /auth/plex/add/poll',
      'POST /auth/logout',
      'DELETE /api/plex/admins/42',
    ])
    expect(calls[3].body).toEqual({ pinId: 1 })
    expect(calls[4].body).toEqual({ pinId: 2 })
  })
})
