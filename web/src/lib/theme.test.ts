import { afterEach, describe, expect, it, vi } from 'vitest'
import { mockColorScheme } from '@/test/match-media'
import { applyTheme, isPreference, prefersDark, readPreference, resolveTheme, savePreference, THEME_KEY } from './theme'

afterEach(() => vi.unstubAllGlobals())

describe('isPreference', () => {
  it('accepts exactly the three choices', () => {
    for (const v of ['light', 'dark', 'system']) expect(isPreference(v)).toBe(true)
    for (const v of ['', 'Dark', 'auto', null, undefined, 1, {}]) expect(isPreference(v)).toBe(false)
  })
})

describe('readPreference', () => {
  const store = (v: string | null) => ({ getItem: () => v })

  it('returns what was saved', () => {
    for (const v of ['light', 'dark', 'system'] as const) expect(readPreference(store(v))).toBe(v)
  })
  it('falls back to "system" for nothing, junk, or storage that throws', () => {
    expect(readPreference(store(null))).toBe('system')
    expect(readPreference(store('purple'))).toBe('system')
    expect(
      readPreference({
        getItem: () => {
          throw new Error('blocked')
        },
      }),
    ).toBe('system')
    expect(readPreference(undefined)).toBe('system') // no storage at all in this environment is handled the same way
  })
  it('reads the real localStorage by default', () => {
    localStorage.setItem(THEME_KEY, 'light')
    expect(readPreference()).toBe('light')
  })
})

describe('savePreference', () => {
  it('writes under the shared key', () => {
    const set = vi.fn()
    savePreference('dark', { setItem: set })
    expect(set).toHaveBeenCalledWith('antennarr-theme', 'dark')
  })
  it('does not throw when storage is blocked', () => {
    expect(() =>
      savePreference('dark', {
        setItem: () => {
          throw new Error('quota')
        },
      }),
    ).not.toThrow()
  })
  it('round-trips through the real localStorage', () => {
    savePreference('light')
    expect(readPreference()).toBe('light')
  })
})

describe('resolveTheme', () => {
  it('lets an explicit choice win and "system" follow the OS', () => {
    expect(resolveTheme('light', true)).toBe('light')
    expect(resolveTheme('dark', false)).toBe('dark')
    expect(resolveTheme('system', true)).toBe('dark')
    expect(resolveTheme('system', false)).toBe('light')
  })
})

describe('applyTheme', () => {
  it("sets Tailwind's dark class and the browser's colour scheme, and undoes them", () => {
    const root = document.createElement('div')
    applyTheme('dark', root)
    expect(root.classList.contains('dark')).toBe(true)
    expect(root.style.colorScheme).toBe('dark')
    applyTheme('light', root)
    expect(root.classList.contains('dark')).toBe(false)
    expect(root.style.colorScheme).toBe('light')
  })
  it('does not disturb other classes', () => {
    const root = document.createElement('div')
    root.className = 'keep me'
    applyTheme('dark', root)
    applyTheme('light', root)
    expect(root.className).toBe('keep me')
  })
  it('defaults to the document element', () => {
    applyTheme('dark')
    expect(document.documentElement).toHaveClass('dark')
  })
})

describe('prefersDark', () => {
  it('asks the browser', () => {
    mockColorScheme(true)
    expect(prefersDark()).toBe(true)
    mockColorScheme(false)
    expect(prefersDark()).toBe(false)
  })
  it('says no where there is no matchMedia', () => {
    vi.stubGlobal('matchMedia', undefined)
    const original = window.matchMedia
    // @ts-expect-error simulating an environment without matchMedia
    window.matchMedia = undefined
    expect(prefersDark()).toBe(false)
    window.matchMedia = original
  })
})
