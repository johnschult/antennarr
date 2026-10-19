import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { parseChangelog } from '@/lib/changelog'
import { WhatsNewDialog } from './whats-new-dialog'

const releases = parseChangelog(`## 2026.10.5 (2026-10-18)

Yagi 2026.10.5 in a line.

### Read before updating

- **Media servers** see the tuner's own name. See [the docs](https://example.com/docs).

### Added

- A \`thing\` with *style*.
- [Not safe](javascript:void) at all.

## 2026.10.4 (2026-10-17)

- Older and plain.
`)

const show = (props: Partial<Parameters<typeof WhatsNewDialog>[0]> = {}) => {
  const onClose = vi.fn()
  render(<WhatsNewDialog open onClose={onClose} version="2026.10.5" releases={releases.slice(0, 1)} updated {...props} />)
  return { onClose, user: userEvent.setup() }
}

describe('WhatsNewDialog', () => {
  it('is titled with the version, says it was updated, and shows the notes', () => {
    show()
    expect(screen.getByRole('dialog', { name: "What's new in Yagi 2026.10.5" })).toBeInTheDocument()
    expect(screen.getByText('Yagi was updated. Here is what changed.')).toBeInTheDocument()
    expect(screen.getByText('Yagi 2026.10.5 in a line.')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Read before updating' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Added' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(3) // one item in the first group, two in the second
  })

  it('draws the Markdown in the items, and only makes safe links', () => {
    show()
    expect(screen.getByText('Media servers').tagName).toBe('STRONG')
    expect(screen.getByText('thing').tagName).toBe('CODE')
    expect(screen.getByText('style').tagName).toBe('EM')
    expect(screen.getByRole('link', { name: 'the docs' })).toHaveAttribute('href', 'https://example.com/docs')
    expect(screen.queryByRole('link', { name: 'Not safe' })).not.toBeInTheDocument()
    expect(screen.getByText('Not safe', { exact: false })).toBeInTheDocument()
  })

  it('has the date of a single release without repeating its number, which is in the title', () => {
    show()
    expect(screen.getByText('18 Oct 2026')).toBeInTheDocument()
    expect(within(screen.getByRole('region', { name: 'Release notes' })).queryByText('2026.10.5')).not.toBeInTheDocument()
  })

  it('names each release when several went by, newest first, and says so', () => {
    show({ releases })
    expect(screen.getByText('Yagi was updated. Here is what changed since you last looked.')).toBeInTheDocument()
    const headings = within(screen.getByRole('region', { name: 'Release notes' })).getAllByRole('heading', { level: 3 })
    expect(headings.map((h) => h.textContent)).toEqual(['2026.10.518 Oct 2026', '2026.10.417 Oct 2026'])
    expect(screen.getByText('Older and plain.')).toBeInTheDocument()
  })

  it('is plainer when someone asked for it from the footer', () => {
    show({ updated: false })
    expect(screen.getByText('The release notes for this version.')).toBeInTheDocument()
  })

  it('closes with Got it!, the X and Escape', async () => {
    const a = show()
    await a.user.click(screen.getByRole('button', { name: 'Got it!' }))
    expect(a.onClose).toHaveBeenCalledTimes(1)
  })
  it('closes with the X', async () => {
    const a = show()
    await a.user.click(screen.getByRole('button', { name: 'Close' }))
    expect(a.onClose).toHaveBeenCalled()
  })
  it('closes with Escape', async () => {
    const a = show()
    await a.user.keyboard('{Escape}')
    expect(a.onClose).toHaveBeenCalled()
  })

  it('has the notes in a region a keyboard can scroll, and a link to every release', () => {
    show()
    expect(screen.getByRole('region', { name: 'Release notes' })).toHaveAttribute('tabindex', '0')
    const all = screen.getByRole('link', { name: /All releases/ })
    expect(all).toHaveAttribute('href', 'https://github.com/johnschult/yagi/releases')
    expect(all).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('is not there when closed', () => {
    show({ open: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
