import { afterEach, describe, expect, it, vi } from 'vitest'
// The script is read as text so the test runs exactly what the browser will.
import script from '../../public/theme-init.js?raw'
import { mockColorScheme } from '@/test/match-media'
import { resolveTheme, THEME_KEY, type ThemePreference } from './theme'

// public/theme-init.js runs before the app loads, to avoid a flash of the wrong
// theme. It can't import the app's code, so this keeps the two in step.
const run = () => new Function(script)()
const html = document.documentElement

afterEach(() => vi.unstubAllGlobals())

describe('theme-init.js', () => {
  it('agrees with the app for every choice and every OS setting', () => {
    for (const pref of ['light', 'dark', 'system'] as ThemePreference[]) {
      for (const osDark of [true, false]) {
        html.className = ''
        html.style.colorScheme = ''
        localStorage.setItem(THEME_KEY, pref)
        mockColorScheme(osDark)
        run()
        const want = resolveTheme(pref, osDark)
        expect(html.classList.contains('dark'), `${pref} with dark OS=${osDark}`).toBe(want === 'dark')
        expect(html.style.colorScheme).toBe(want)
      }
    }
  })

  it('treats nothing saved, or junk, as "system"', () => {
    mockColorScheme(true)
    run()
    expect(html).toHaveClass('dark')
    html.className = ''
    localStorage.setItem(THEME_KEY, 'sepia')
    run()
    expect(html).toHaveClass('dark')
    html.className = ''
    mockColorScheme(false)
    run()
    expect(html).not.toHaveClass('dark')
  })

  it('stays out of the way when storage is blocked', () => {
    mockColorScheme(true)
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(run).not.toThrow() // the app sets the theme itself once it loads
    vi.restoreAllMocks()
  })

  it('uses the same storage key as the app', () => {
    expect(script).toContain(`'${THEME_KEY}'`)
  })
})
