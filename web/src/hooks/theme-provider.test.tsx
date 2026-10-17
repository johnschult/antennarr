import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockColorScheme } from '@/test/match-media'
import { THEME_KEY } from '@/lib/theme'
import { useTheme } from './theme-context'
import { ThemeProvider } from './theme-provider'

afterEach(() => vi.unstubAllGlobals())

function Probe() {
  const { preference, resolved, setPreference } = useTheme()
  return (
    <div>
      <p data-testid="state">
        {preference}/{resolved}
      </p>
      {(['light', 'dark', 'system'] as const).map((p) => (
        <button key={p} onClick={() => setPreference(p)}>
          choose {p}
        </button>
      ))}
    </div>
  )
}

const mount = () =>
  render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>,
  )
const state = () => screen.getByTestId('state').textContent
const html = document.documentElement

describe('ThemeProvider', () => {
  it('starts on "system" and follows a dark OS', () => {
    mockColorScheme(true)
    mount()
    expect(state()).toBe('system/dark')
    expect(html).toHaveClass('dark')
    expect(html.style.colorScheme).toBe('dark')
  })

  it('starts light on a light OS', () => {
    mockColorScheme(false)
    mount()
    expect(state()).toBe('system/light')
    expect(html).not.toHaveClass('dark')
    expect(html.style.colorScheme).toBe('light')
  })

  it('uses a saved choice in preference to the OS', () => {
    mockColorScheme(true)
    localStorage.setItem(THEME_KEY, 'light')
    mount()
    expect(state()).toBe('light/light')
    expect(html).not.toHaveClass('dark')
  })

  it('ignores a saved value that is not a theme', () => {
    mockColorScheme(false)
    localStorage.setItem(THEME_KEY, 'sepia')
    mount()
    expect(state()).toBe('system/light')
  })

  it('changes the page and remembers the choice', async () => {
    mockColorScheme(false)
    const user = userEvent.setup()
    mount()

    await user.click(screen.getByText('choose dark'))
    expect(state()).toBe('dark/dark')
    expect(html).toHaveClass('dark')
    expect(localStorage.getItem(THEME_KEY)).toBe('dark')

    await user.click(screen.getByText('choose light'))
    expect(state()).toBe('light/light')
    expect(html).not.toHaveClass('dark')
    expect(localStorage.getItem(THEME_KEY)).toBe('light')
  })

  it('follows the OS live while on "system", and only then', async () => {
    const os = mockColorScheme(false)
    const user = userEvent.setup()
    mount()

    act(() => os.setDark(true))
    expect(state()).toBe('system/dark')
    expect(html).toHaveClass('dark')

    await user.click(screen.getByText('choose light'))
    act(() => os.setDark(true)) // an explicit choice must not be overruled by the OS
    expect(state()).toBe('light/light')
    expect(html).not.toHaveClass('dark')

    await user.click(screen.getByText('choose system'))
    expect(state()).toBe('system/dark') // back to following it
  })

  it('stops listening to the OS when removed', () => {
    const os = mockColorScheme(false)
    const { unmount } = mount()
    expect(os.listenerCount()).toBe(1)
    unmount()
    expect(os.listenerCount()).toBe(0)
  })

  it('picks up a change made in another tab', () => {
    mockColorScheme(false)
    mount()
    localStorage.setItem(THEME_KEY, 'dark')
    act(() => {
      window.dispatchEvent(new StorageEvent('storage', { key: THEME_KEY, newValue: 'dark' }))
    })
    expect(state()).toBe('dark/dark')
    expect(html).toHaveClass('dark')
  })

  it('still works when storage is blocked', async () => {
    mockColorScheme(false)
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const user = userEvent.setup()
    mount()
    await user.click(screen.getByText('choose dark'))
    expect(state()).toBe('dark/dark') // applied for this visit even though it can't be saved
    expect(html).toHaveClass('dark')
    vi.restoreAllMocks()
  })

  it('works in a browser without matchMedia', () => {
    vi.stubGlobal('matchMedia', undefined)
    // @ts-expect-error simulating an environment without matchMedia
    window.matchMedia = undefined
    mount()
    expect(state()).toBe('system/light')
  })
})

describe('useTheme without a provider', () => {
  it('gives a harmless dark default, so components work in isolation', () => {
    render(<Probe />)
    expect(state()).toBe('dark/dark')
  })
})
