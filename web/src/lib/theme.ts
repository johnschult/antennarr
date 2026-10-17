// Dark / light / system theme. The choice is kept in localStorage; "system"
// follows the operating system's setting and changes with it.

export type ThemePreference = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

export const THEME_KEY = 'antennarr-theme'
export const PREFERENCES: ThemePreference[] = ['light', 'dark', 'system']

export const isPreference = (v: unknown): v is ThemePreference => v === 'light' || v === 'dark' || v === 'system'

/** What was saved, or "system" if nothing valid was (or storage is blocked). */
export function readPreference(storage: Pick<Storage, 'getItem'> | undefined = safeStorage()): ThemePreference {
  try {
    const v = storage?.getItem(THEME_KEY)
    return isPreference(v) ? v : 'system'
  } catch {
    return 'system'
  }
}

export function savePreference(p: ThemePreference, storage: Pick<Storage, 'setItem'> | undefined = safeStorage()): void {
  try {
    storage?.setItem(THEME_KEY, p)
  } catch {
    // private mode or blocked storage: the choice just won't be remembered
  }
}

export function resolveTheme(p: ThemePreference, systemPrefersDark: boolean): ResolvedTheme {
  return p === 'system' ? (systemPrefersDark ? 'dark' : 'light') : p
}

/** Put the theme on the page: Tailwind's `dark` class and the browser's colour scheme. */
export function applyTheme(t: ResolvedTheme, root: HTMLElement = document.documentElement): void {
  root.classList.toggle('dark', t === 'dark')
  root.style.colorScheme = t
}

export const prefersDark = (): boolean =>
  typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches

// Touching localStorage can throw (blocked cookies, sandboxed frames).
function safeStorage(): Storage | undefined {
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage
  } catch {
    return undefined
  }
}
