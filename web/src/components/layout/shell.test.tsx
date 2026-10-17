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
    expect(menu.getAllByRole('link').map((l) => l.textContent)).toEqual(['Live', 'Viewers', 'Setup', 'Tuner', 'Settings'])
  })

  it('also holds the tuner status and sign-out that the phone header has no room for', async () => {
    mount()
    const user = userEvent.setup()
    await user.click(button())
    const panel = document.getElementById('mobile-menu')!
    expect(within(panel).getByText('Steve FLEX 4K', { exact: false })).toBeInTheDocument()
    expect(within(panel).getByTestId('connection')).toBeInTheDocument()
    expect(await within(panel).findByRole('button', { name: /john/ })).toBeInTheDocument()
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
