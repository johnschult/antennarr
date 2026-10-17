import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '@/hooks/theme-provider'
import { THEME_KEY } from '@/lib/theme'
import { mockColorScheme } from '@/test/match-media'
import { ThemeToggle } from './theme-toggle'

afterEach(() => vi.unstubAllGlobals())

const mount = () =>
  render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  )

describe('ThemeToggle', () => {
  it('has a button for each choice, in a labelled group', () => {
    mockColorScheme(false)
    mount()
    const group = screen.getByRole('group', { name: 'Theme' })
    expect(group).toBeInTheDocument()
    expect(screen.getAllByRole('button').map((b) => b.getAttribute('aria-label'))).toEqual(['Light', 'Dark', 'System'])
  })

  it('shows the current choice as pressed, starting with System', () => {
    mockColorScheme(true)
    mount()
    expect(screen.getByRole('button', { name: 'System' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Light' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('reflects a saved choice', () => {
    mockColorScheme(false)
    localStorage.setItem(THEME_KEY, 'dark')
    mount()
    expect(screen.getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('switches the page and moves the pressed state when clicked', async () => {
    mockColorScheme(false)
    const user = userEvent.setup()
    mount()

    await user.click(screen.getByRole('button', { name: 'Dark' }))
    expect(document.documentElement).toHaveClass('dark')
    expect(screen.getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'System' })).toHaveAttribute('aria-pressed', 'false')
    expect(localStorage.getItem(THEME_KEY)).toBe('dark')

    await user.click(screen.getByRole('button', { name: 'Light' }))
    expect(document.documentElement).not.toHaveClass('dark')
    expect(screen.getByRole('button', { name: 'Light' })).toHaveAttribute('aria-pressed', 'true')

    await user.click(screen.getByRole('button', { name: 'System' }))
    expect(localStorage.getItem(THEME_KEY)).toBe('system')
  })

  it('can be used from the keyboard', async () => {
    mockColorScheme(false)
    const user = userEvent.setup()
    mount()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Light' })).toHaveFocus()
    await user.keyboard('{Tab}{Enter}')
    expect(screen.getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'true')
  })
})
