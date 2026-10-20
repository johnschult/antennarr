import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { makeState } from '@/test/fixtures'
import { mockPhone } from '@/test/match-media'
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

describe('Shell bottom bar on a phone', () => {
  const mount = (route = '/') => {
    mockPhone(true)
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn: { id: 7, name: 'john' }, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(), { route })
  }
  const bar = () => within(screen.getByRole('navigation', { name: 'Main' }))

  it('has every page along the bottom, each with an icon and its name, and no menu button', () => {
    mount()
    expect(bar().getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Tuners', 'Activity', 'Settings'])
    expect(bar().getAllByRole('link').every((l) => l.querySelector('svg'))).toBe(true)
    expect(screen.queryByRole('button', { name: /menu/i })).not.toBeInTheDocument()
    expect(screen.getByRole('navigation', { name: 'Main' }).className).toContain('fixed')
    expect(screen.getByRole('navigation', { name: 'Main' }).className).toContain('bottom-0')
  })

  it('is the only navigation, not one more beside the wide-screen top bar', () => {
    mount()
    expect(screen.getAllByRole('navigation', { name: 'Main' })).toHaveLength(1)
    expect(within(screen.getByRole('banner')).queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('marks the page you are on', () => {
    mount('/viewers')
    expect(bar().getByRole('link', { name: 'Viewers' })).toHaveAttribute('aria-current', 'page')
    expect(bar().getByRole('link', { name: 'Live' })).not.toHaveAttribute('aria-current')
  })

  it('leaves room for the bar under the page, and for the home indicator', () => {
    mount()
    expect(screen.getByRole('contentinfo').parentElement!.className).toContain('pb-[calc(4rem+env(safe-area-inset-bottom))]')
    expect(screen.getByRole('navigation', { name: 'Main' }).className).toContain('pb-[env(safe-area-inset-bottom)]')
  })

  it('keeps the connection status in the header, and moves sign-out, which the phone header has no room for, to the footer', async () => {
    mount()
    const footer = within(screen.getByRole('contentinfo'))
    expect(within(screen.getByRole('banner')).getByTestId('connection')).toBeInTheDocument()
    expect(footer.queryByTestId('connection')).not.toBeInTheDocument()
    expect(await footer.findByRole('button', { name: /john/ })).toBeInTheDocument()
    expect(within(screen.getByRole('banner')).queryByRole('button', { name: /john/ })).not.toBeInTheDocument()
  })

  it('goes to a page when you tap it', async () => {
    mount()
    await userEvent.setup().click(bar().getByRole('link', { name: 'Settings' }))
    expect(bar().getByRole('link', { name: 'Settings' })).toHaveAttribute('aria-current', 'page')
  })

  it('does not offer Activity when the history is off', () => {
    mockPhone(true)
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    const s = makeState()
    s.settings.history = false
    renderWithApp(<Shell />, s)
    expect(bar().queryByRole('link', { name: 'Activity' })).not.toBeInTheDocument()
    expect(bar().getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Tuners', 'Settings'])
  })
})

describe('Shell top bar on a wide screen', () => {
  const mount = (route = '/') => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(), { route })
  }

  it('shows an icon in the top bar only beside the page you are on', () => {
    mount('/viewers')
    const bar = within(screen.getByRole('navigation', { name: 'Main' }))
    expect(bar.getAllByRole('link').filter((l) => l.querySelector('svg')).map((l) => l.textContent)).toEqual(['Viewers'])
  })

  it('has no menu button and no bottom bar', () => {
    mount()
    expect(screen.queryByRole('button', { name: /menu/i })).not.toBeInTheDocument()
    expect(screen.getAllByRole('navigation', { name: 'Main' })).toHaveLength(1)
    expect(within(screen.getByRole('banner')).getByRole('navigation', { name: 'Main' })).toBeInTheDocument()
  })
})

describe('Shell logo', () => {
  const mount = (route: string) => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(), { route })
  }

  it('is a link to the Live page, named for the app', () => {
    mount('/tuner')
    const link = screen.getByRole('link', { name: 'Yagi' })
    expect(link).toHaveAttribute('href', '/')
    expect(link).toHaveAttribute('title', 'Live')
    expect(link.querySelector('img')).toHaveAttribute('aria-hidden', 'true') // the name comes from the word, not the picture
  })

  it('is not one of the pages in the menu, which are only the pages', () => {
    mount('/')
    expect(within(screen.getByRole('navigation', { name: 'Main' })).queryByRole('link', { name: 'Yagi' })).not.toBeInTheDocument()
  })
})

describe('Shell pages', () => {
  const mount = (history: boolean) => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    const s = makeState()
    s.settings.history = history
    return renderWithApp(<Shell />, s)
  }

  it('offers Activity while the history is kept', () => {
    mount(true)
    expect(within(screen.getByRole('navigation', { name: 'Main' })).getByRole('link', { name: 'Activity' })).toBeInTheDocument()
  })

  it('does not offer Activity when the history is off', () => {
    mount(false)
    expect(within(screen.getByRole('navigation', { name: 'Main' })).queryByRole('link', { name: 'Activity' })).not.toBeInTheDocument()
    expect(within(screen.getByRole('navigation', { name: 'Main' })).getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Tuners', 'Settings'])
  })
})

describe('Shell footer', () => {
  const mount = (over: Parameters<typeof makeState>[0] = {}) => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: false, signedIn: null, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(over))
  }

  it('has the GitHub icon linking to the project', () => {
    mount()
    const gh = screen.getByRole('link', { name: 'Yagi on GitHub' })
    expect(gh).toHaveAttribute('href', 'https://github.com/johnschult/yagi')
    expect(gh.querySelector('svg')).not.toBeNull() // an icon, not text
    expect(gh.querySelector('svg')).toHaveAttribute('aria-hidden', 'true') // the link's name comes from the label, not the drawing
  })

  it('shows the version in the footer, first, linked to its release page, and not in the top bar where the wordmark would cover it', () => {
    mount()
    const version = screen.getByTestId('version')
    expect(version).toHaveTextContent('v2026.10.0')
    expect(version).toHaveAttribute('href', 'https://github.com/johnschult/yagi/releases/tag/v2026.10.0')
    expect(within(screen.getByRole('contentinfo')).getByTestId('version')).toBe(version)
    expect(within(screen.getByRole('banner')).queryByTestId('version')).not.toBeInTheDocument()
    expect(within(screen.getByRole('contentinfo')).getAllByRole('link')[0]).toBe(version)
  })

  it('says the tagline in the footer, with anywhere in bold', () => {
    mount()
    const footer = within(screen.getByRole('contentinfo'))
    expect(footer.getByText('anywhere').tagName).toBe('STRONG')
    expect(screen.getByRole('contentinfo')).toHaveTextContent('Point your antenna anywhere.')
  })

  it('centers the footer links', () => {
    mount()
    const footer = screen.getByRole('contentinfo')
    expect(footer.className).toContain('justify-center')
    expect(within(footer).getAllByRole('link').map((l) => l.textContent || l.getAttribute('aria-label'))).toEqual(['v2026.10.0', 'Logs', 'Help', 'Yagi on GitHub'])
  })

  it('keeps the footer at the bottom of a short page: the page is a full-height column and the content takes the spare room', () => {
    mount()
    const footer = screen.getByRole('contentinfo')
    const page = footer.parentElement!
    expect(page.className).toMatch(/\bflex\b/)
    expect(page.className).toMatch(/flex-col/)
    expect(page.className).toMatch(/min-h-dvh/)
    expect(screen.getByRole('main').className).toMatch(/flex-1/)
    expect(page.lastElementChild).toBe(footer) // after the main area, so it ends up under it
  })

  it('has a Help link to the documentation for this version', () => {
    mount()
    const help = screen.getByRole('link', { name: 'Help' })
    expect(help).toHaveAttribute('href', 'https://github.com/johnschult/yagi/blob/v2026.10.0/docs/README.md')
  })

  it('points Help at the latest docs for a build from a checkout', () => {
    mount({ version: 'dev+3553344', versionUrl: 'https://github.com/johnschult/yagi', docsUrl: 'https://github.com/johnschult/yagi/blob/main/docs/README.md' })
    expect(screen.getByRole('link', { name: 'Help' })).toHaveAttribute('href', 'https://github.com/johnschult/yagi/blob/main/docs/README.md')
  })

  it('opens all three in a new tab, without handing the page to the other site', () => {
    mount()
    for (const link of [screen.getByTestId('version'), screen.getByRole('link', { name: 'Help' }), screen.getByRole('link', { name: 'Yagi on GitHub' })]) {
      expect(link).toHaveAttribute('target', '_blank')
      expect(link.getAttribute('rel')).toMatch(/noopener/)
      expect(link.getAttribute('rel')).toMatch(/noreferrer/)
    }
  })

  it('says dev for a build from a checkout, linked to the project', () => {
    mount({ version: 'dev+3553344', versionUrl: 'https://github.com/johnschult/yagi' })
    expect(screen.getByTestId('version')).toHaveTextContent('dev+3553344')
    expect(screen.getByTestId('version')).toHaveAttribute('href', 'https://github.com/johnschult/yagi')
  })
})
