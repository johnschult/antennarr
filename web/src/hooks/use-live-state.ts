import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { API, getState } from '@/lib/api'
import type { AppState } from '@/lib/types'

export const STATE_KEY = ['state'] as const

/**
 * The app state, kept fresh by the server's event stream. If the stream drops
 * (EventSource reconnects on its own), fall back to polling meanwhile.
 */
export function useLiveState() {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const es = new EventSource(`${API}/events`)
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.addEventListener('state', (e) => {
      setConnected(true)
      qc.setQueryData<AppState>(STATE_KEY, JSON.parse((e as MessageEvent<string>).data))
    })
    return () => es.close()
  }, [qc])

  const query = useQuery({
    queryKey: STATE_KEY,
    queryFn: getState,
    refetchInterval: connected ? false : 5000,
    refetchOnWindowFocus: false,
    retry: false,
  })

  return { state: query.data, connected, error: query.error, isLoading: query.isLoading }
}
