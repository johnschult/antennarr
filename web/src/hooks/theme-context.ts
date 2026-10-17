import { createContext, useContext } from 'react'
import type { ResolvedTheme, ThemePreference } from '@/lib/theme'

export interface ThemeValue {
  /** What the person chose. */
  preference: ThemePreference
  /** What is actually showing, after "system" is looked up. */
  resolved: ResolvedTheme
  setPreference: (p: ThemePreference) => void
}

// Without a provider (in tests, say) the theme is simply the dark default.
export const ThemeContext = createContext<ThemeValue>({ preference: 'dark', resolved: 'dark', setPreference: () => {} })

export const useTheme = () => useContext(ThemeContext)
