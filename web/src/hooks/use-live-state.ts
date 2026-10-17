import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { API, getState } from '@/lib/api'
import type { AppState } from '@/lib/types'

export const STATE_KEY = ['state'] as const

const RETRY_MIN = 3_000
const RETRY_MAX = 15_000
const CLOSED = 2 // EventSource.readyState once the browser has given up

/**
 * The app state, kept fresh by the server's event stream. If the stream drops
 * (EventSource reconnects on its own), fall back to polling meanwhile.
 *
 * The browser only retries by itself after a lost connection. After an error
 * reply (a 403 because a Plex sign-in ended when the server restarted, a 502
 * from a proxy that was restarting) it gives up for good, which used to leave
 * the page on "Reconnecting…" until it was reloaded. So when it gives up, open
 * a new stream ourselves, a little slower each time.
 */
export function useLiveState() {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    let es: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let stopped = false
    let delay = RETRY_MIN

    const open = () => {
      const mine = new EventSource(`${API}/events`)
      es = mine
      mine.onopen = () => {
        delay = RETRY_MIN
        setConnected(true)
      }
      mine.onerror = () => {
        setConnected(false)
        if (mine.readyState === CLOSED && !stopped) {
          timer = setTimeout(open, delay)
          delay = Math.min(delay * 2, RETRY_MAX)
        }
      }
      mine.addEventListener('state', (e) => {
        delay = RETRY_MIN
        setConnected(true)
        qc.setQueryData<AppState>(STATE_KEY, JSON.parse((e as MessageEvent<string>).data))
      })
    }
    open()

    return () => {
      stopped = true
      clearTimeout(timer)
      es?.close()
    }
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
