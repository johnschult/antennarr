import { createContext, useContext } from 'react'
import type { AppState } from '@/lib/types'

export interface AppValue {
  state: AppState
  /** Whether the live event stream is connected (otherwise we are polling). */
  connected: boolean
}

export const AppContext = createContext<AppValue | null>(null)

export function useApp(): AppValue {
  const v = useContext(AppContext)
  if (!v) throw new Error('useApp must be used inside AppStateProvider')
  return v
}
