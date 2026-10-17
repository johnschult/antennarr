import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, addViewer, deleteViewer, getState, patchSettings, replaceViewer } from './api'
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

  it('escapes viewer names in the path and passes the open-access confirmation', async () => {
    const { fn, calls } = stubFetch({
      'PUT /viewers/Mom%20%26%20Dad': () => ({ body: {} }),
      'DELETE /viewers/Mom%20%26%20Dad': () => ({ status: 204 }),
      'DELETE /viewers/Mom%20%26%20Dad?allowOpen=true': () => ({ status: 204 }),
    })
    vi.stubGlobal('fetch', fn)
    await replaceViewer('Mom & Dad', { name: 'Mom & Dad', addresses: ['1.2.3.4'], limit: 1 })
    await expect(deleteViewer('Mom & Dad')).resolves.toBeUndefined() // 204: no body to parse
    await deleteViewer('Mom & Dad', true)
    expect(calls.map((c) => `${c.method} ${c.url.replace(/^.*api/, '')}`)).toEqual([
      'PUT /viewers/Mom%20%26%20Dad',
      'DELETE /viewers/Mom%20%26%20Dad',
      'DELETE /viewers/Mom%20%26%20Dad?allowOpen=true',
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
    expect(err).toMatchObject({ status: 0, message: 'Could not reach Antennarr' })
  })
})
