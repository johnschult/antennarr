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

describe('Shell footer', () => {
  const mount = (over: Parameters<typeof makeState>[0] = {}) => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(over))
  }

  it('shows the version, linked to its release page, and the GitHub icon linking to the project', () => {
    mount()
    const version = screen.getByTestId('version')
    expect(version).toHaveTextContent('2026.10.0')
    expect(version).toHaveAttribute('href', 'https://github.com/johnschult/antennarr/releases/tag/v2026.10.0')
    const gh = screen.getByRole('link', { name: 'Antennarr on GitHub' })
    expect(gh).toHaveAttribute('href', 'https://github.com/johnschult/antennarr')
    expect(gh.querySelector('svg')).not.toBeNull() // an icon, not text
    expect(gh.querySelector('svg')).toHaveAttribute('aria-hidden', 'true') // the link's name comes from the label, not the drawing
  })

  it('opens both in a new tab, without handing the page to the other site', () => {
    mount()
    for (const link of [screen.getByTestId('version'), screen.getByRole('link', { name: 'Antennarr on GitHub' })]) {
      expect(link).toHaveAttribute('target', '_blank')
      expect(link.getAttribute('rel')).toMatch(/noopener/)
      expect(link.getAttribute('rel')).toMatch(/noreferrer/)
    }
  })

  it('says dev for a build from a checkout, linked to the project', () => {
    mount({ version: 'dev+3553344', versionUrl: 'https://github.com/johnschult/antennarr' })
    expect(screen.getByTestId('version')).toHaveTextContent('dev+3553344')
    expect(screen.getByTestId('version')).toHaveAttribute('href', 'https://github.com/johnschult/antennarr')
  })
})
