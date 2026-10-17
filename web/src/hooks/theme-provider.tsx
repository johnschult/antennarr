import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { applyTheme, prefersDark, readPreference, resolveTheme, savePreference, type ThemePreference } from '@/lib/theme'
import { ThemeContext } from './theme-context'

/** Keeps the page's theme in step with the saved choice and, for "system", the OS. */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(() => readPreference())
  const [systemDark, setSystemDark] = useState(prefersDark)

  // Follow the operating system while it is changed under us.
  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const resolved = resolveTheme(preference, systemDark)
  useEffect(() => applyTheme(resolved), [resolved])

  // Another tab changing the choice should change this one too.
  useEffect(() => {
    const onStorage = () => setPreferenceState(readPreference())
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setPreference = useCallback((p: ThemePreference) => {
    setPreferenceState(p)
    savePreference(p)
  }, [])

  const value = useMemo(() => ({ preference, resolved, setPreference }), [preference, resolved, setPreference])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
