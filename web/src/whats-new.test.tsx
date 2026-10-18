import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from '@/App'
import { SEEN_KEY } from '@/hooks/use-whats-new'
import { makeState } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'

// These use the real CHANGELOG.md, bundled into the page, so the notes that appear are the ones that ship.
const open = (version: string) => renderWithApp(<App />, makeState({ version }), { route: '/viewers' })
const dialog = () => screen.findByRole('dialog', { name: /What's new in Antennarr/ })

describe('what is new, after an update', () => {
  it('pops up with the release notes the first time a newer version is opened, and not again once dismissed', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.4')
    const first = open('2026.10.5')
    const d = await dialog()
    expect(d).toHaveAccessibleName("What's new in Antennarr 2026.10.5")
    expect(await within(d).findByRole('heading', { name: 'Read before updating' })).toBeInTheDocument()
    expect(within(d).getByText(/Media servers now always see the tuner's own name/)).toBeInTheDocument()

    await userEvent.setup().click(within(d).getByRole('button', { name: 'Got it!' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(localStorage.getItem(SEEN_KEY)).toBe('2026.10.5')
    first.unmount()

    open('2026.10.5') // the next visit
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('shows everything since the last version seen, when several went by', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.2')
    open('2026.10.5')
    const d = await dialog()
    const versions = (await within(d).findAllByRole('heading', { level: 3 })).map((h) => h.textContent?.slice(0, 9))
    expect(versions).toEqual(['2026.10.5', '2026.10.4', '2026.10.3'])
  })

  it('says nothing to a browser that has never been here, so a new install is not shown its history', async () => {
    open('2026.10.5')
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(localStorage.getItem(SEEN_KEY)).toBe('2026.10.5')
  })

  it("never pops up for a build from a checkout, which has no What's new link either", async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.1')
    open('dev+3553344')
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: "What's new" })).not.toBeInTheDocument()
  })

  it("opens the notes for this version from the footer's What's new, whenever you like", async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.5')
    open('2026.10.5')
    const user = userEvent.setup()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await user.click(within(screen.getByRole('contentinfo')).getByRole('button', { name: "What's new" }))
    const d = await dialog()
    expect(within(d).getByText('The release notes for this version.')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })
})
