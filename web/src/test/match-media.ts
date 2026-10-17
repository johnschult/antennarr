import { vi } from 'vitest'

/**
 * Stand in for the browser's prefers-color-scheme. jsdom has no matchMedia, so
 * this installs one that answers for the dark query and lets a test flip the OS.
 */
export function mockColorScheme(initialDark: boolean) {
  let dark = initialDark
  const listeners = new Set<(e: MediaQueryListEvent) => void>()
  const list = {
    get matches() {
      return dark
    },
    media: '(prefers-color-scheme: dark)',
    addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => listeners.delete(fn),
  }
  vi.stubGlobal('matchMedia', (q: string) => (q === '(prefers-color-scheme: dark)' ? list : { ...list, matches: false, media: q }))
  window.matchMedia = globalThis.matchMedia
  return {
    setDark(next: boolean) {
      dark = next
      listeners.forEach((fn) => fn({ matches: next } as MediaQueryListEvent))
    },
    listenerCount: () => listeners.size,
  }
}

/**
 * Make the window phone-sized (or not) for tests of the layouts that switch at the
 * `md` breakpoint. Answers only the phone query; other queries don't match.
 */
export function mockPhone(initial: boolean) {
  let phone = initial
  const listeners = new Set<() => void>()
  vi.stubGlobal('matchMedia', (q: string) => ({
    get matches() {
      return q === '(max-width: 767px)' && phone
    },
    media: q,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  }))
  window.matchMedia = globalThis.matchMedia
  return {
    set(next: boolean) {
      phone = next
      listeners.forEach((fn) => fn())
    },
  }
}
