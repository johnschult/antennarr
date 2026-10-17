import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { makeState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { Shell } from './shell'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const status = (signedIn: { id: number; name: string } | null) => ({
  'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn, admin: true } }),
})

describe('Shell sign-in chip', () => {
  it('shows nothing when no one is signed in with Plex', async () => {
    const { fn, calls } = stubFetch(status(null))
    vi.stubGlobal('fetch', fn)
    renderWithApp(<Shell />, makeState())
    await waitFor(() => expect(calls.length).toBeGreaterThan(0))
    expect(screen.queryByRole('button', { name: /Sign out/ })).not.toBeInTheDocument()
  })

  it('shows who is signed in, and signing out tells the server then reloads', async () => {
    const reload = vi.fn()
    vi.stubGlobal('location', { ...window.location, reload })
    const { fn, calls } = stubFetch({ ...status({ id: 7, name: 'john' }), 'POST /auth/logout': () => ({ body: {} }) })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<Shell />, makeState())
    await userEvent.setup().click(await screen.findByRole('button', { name: /john/ }))
    await waitFor(() => expect(reload).toHaveBeenCalled())
    expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/auth/logout'))).toBe(true)
  })
})
