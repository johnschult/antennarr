import { screen, waitFor, within } from '@testing-library/react'
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

describe('Shell menu on a phone', () => {
  const mount = (route = '/') => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /auth/status': () => ({ body: { plexEnabled: true, signedIn: { id: 7, name: 'john' }, admin: true } }) }).fn)
    return renderWithApp(<Shell />, makeState(), { route })
  }
  const button = () => screen.getByRole('button', { name: /menu/i })

  it('starts closed, with a button that says it is closed', () => {
    mount()
    expect(button()).toHaveAttribute('aria-expanded', 'false')
    expect(button()).toHaveAccessibleName('Open menu')
    expect(screen.queryByRole('navigation', { name: 'Main menu' })).not.toBeInTheDocument() // hidden until opened
  })

  it('opens to every page, and says it is open', async () => {
    mount()
    await userEvent.setup().click(button())
    expect(button()).toHaveAttribute('aria-expanded', 'true')
    expect(button()).toHaveAccessibleName('Close menu')
    const menu = within(screen.getByRole('navigation', { name: 'Main menu' }))
    expect(menu.getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Tuners', 'Activity', 'Settings'])
  })

  it('also holds the connection and sign-out that the phone header has no room for, and nothing about the tuners', async () => {
    mount()
    const user = userEvent.setup()
    await user.click(button())
    const panel = document.getElementById('mobile-menu')!
    expect(within(panel).getByTestId('connection')).toBeInTheDocument()
    expect(await within(panel).findByRole('button', { name: /john/ })).toBeInTheDocument()
    expect(within(panel).queryByText(/Steve FLEX 4K/)).not.toBeInTheDocument()
  })

  it('has an icon on every page in the phone menu', async () => {
    mount('/viewers')
    await userEvent.setup().click(button())
    const menu = within(screen.getByRole('navigation', { name: 'Main menu' }))
    expect(menu.getAllByRole('link').every((l) => l.querySelector('svg'))).toBe(true)
  })

  it('shows an icon in the top bar only beside the page you are on', () => {
    mount('/viewers')
    const bar = within(screen.getByRole('navigation', { name: 'Main' }))
    expect(bar.getAllByRole('link').filter((l) => l.querySelector('svg')).map((l) => l.textContent)).toEqual(['Viewers'])
  })

  it('marks the page you are on', async () => {
    mount('/viewers')
    await userEvent.setup().click(button())
    const menu = within(screen.getByRole('navigation', { name: 'Main menu' }))
    expect(menu.getByRole('link', { name: 'Viewers' })).toHaveAttribute('aria-current', 'page')
    expect(menu.getByRole('link', { name: 'Live' })).not.toHaveAttribute('aria-current')
  })

  it('puts itself away when you pick a page', async () => {
    mount()
    const user = userEvent.setup()
    await user.click(button())
    await user.click(within(screen.getByRole('navigation', { name: 'Main menu' })).getByRole('link', { name: 'Settings' }))
    expect(button()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('navigation', { name: 'Main menu' })).not.toBeInTheDocument()
  })

  it('closes with Escape and gives the focus back to its button', async () => {
    mount()
    const user = userEvent.setup()
    await user.click(button())
    await user.keyboard('{Escape}')
    expect(button()).toHaveAttribute('aria-expanded', 'false')
    expect(button()).toHaveFocus()
  })

  it('closes when the button is pressed again', async () => {
    mount()
    const user = userEvent.setup()
    await user.click(button())
    await user.click(button())
    expect(button()).toHaveAttribute('aria-expanded', 'false')
  })

  it('keeps the wide-screen navigation too', () => {
    mount()
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument()
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

  it('does not offer Activity, in the bar or the phone menu, when the history is off', async () => {
    mount(false)
    expect(within(screen.getByRole('navigation', { name: 'Main' })).queryByRole('link', { name: 'Activity' })).not.toBeInTheDocument()
    expect(within(screen.getByRole('navigation', { name: 'Main' })).getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Tuners', 'Settings'])
    await userEvent.setup().click(screen.getByRole('button', { name: /menu/i }))
    expect(within(screen.getByRole('navigation', { name: 'Main menu' })).queryByRole('link', { name: 'Activity' })).not.toBeInTheDocument()
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
    expect(version).toHaveTextContent('2026.10.0')
    expect(version).toHaveAttribute('href', 'https://github.com/johnschult/yagi/releases/tag/v2026.10.0')
    expect(within(screen.getByRole('contentinfo')).getByTestId('version')).toBe(version)
    expect(within(screen.getByRole('banner')).queryByTestId('version')).not.toBeInTheDocument()
    expect(within(screen.getByRole('contentinfo')).getAllByRole('link')[0]).toBe(version)
  })

  it('centers the footer links', () => {
    mount()
    const footer = screen.getByRole('contentinfo')
    expect(footer.className).toContain('justify-center')
    expect(within(footer).getAllByRole('link').map((l) => l.textContent || l.getAttribute('aria-label'))).toEqual(['2026.10.0', 'Logs', 'Help', 'Yagi on GitHub'])
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
