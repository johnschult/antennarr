import { useQuery } from '@tanstack/react-query'
import { getAuthStatus } from '@/lib/api'

export const AUTH_KEY = ['auth'] as const

/** Who this browser is, for signing in: whether Plex sign-in is on, and who is signed in. */
export function useAuthStatus(enabled = true) {
  return useQuery({ queryKey: AUTH_KEY, queryFn: getAuthStatus, enabled, staleTime: 30_000, retry: false, refetchOnWindowFocus: false })
}
