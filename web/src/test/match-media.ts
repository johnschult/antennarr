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
