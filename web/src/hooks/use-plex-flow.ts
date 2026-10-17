import { useCallback, useEffect, useRef, useState } from 'react'
import { pollPlex, startPlex } from '@/lib/api'

export type PlexFlow =
  | { phase: 'idle' }
  | { phase: 'starting' }
  /** The person is signing in on plex.tv; authUrl is the link, in case the window was blocked. */
  | { phase: 'waiting'; authUrl: string }
  | { phase: 'done'; name: string }
  | { phase: 'denied'; name: string }
  | { phase: 'expired' }
  | { phase: 'error'; message: string }

export const POLL_EVERY = 2000

/**
 * Drives "Sign in with Plex": ask the server to start a sign-in, send the
 * person to Plex's own page in a window, then poll the server until they have
 * finished. `add` is for adding an account to the allowed list rather than
 * signing in. The Plex token never reaches the browser.
 */
export function usePlexFlow({ add = false, onDone }: { add?: boolean; onDone?: () => void } = {}) {
  const [flow, setFlow] = useState<PlexFlow>({ phase: 'idle' })
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const popup = useRef<Window | null>(null)
  const run = useRef(0) // bumped on every start and cancel, so a stale poll can tell it was replaced
  const onDoneRef = useRef(onDone)
  onDoneRef.current = onDone

  const stop = useCallback(() => {
    run.current++
    clearTimeout(timer.current)
    if (popup.current && !popup.current.closed) popup.current.close()
    popup.current = null
  }, [])

  useEffect(() => stop, [stop]) // leaving the page abandons the sign-in

  const poll = useCallback(
    (pinId: number, mine: number) => {
      timer.current = setTimeout(async () => {
        if (run.current !== mine) return
        try {
          const r = await pollPlex(pinId, add)
          if (run.current !== mine) return
          switch (r.status) {
            case 'pending':
              return poll(pinId, mine)
            case 'ok':
              popup.current?.close()
              setFlow({ phase: 'done', name: (r.user ?? r.account)?.name ?? '' })
              return onDoneRef.current?.()
            case 'denied':
              popup.current?.close()
              return setFlow({ phase: 'denied', name: r.user.name })
            case 'expired':
              popup.current?.close()
              return setFlow({ phase: 'expired' })
          }
        } catch (err) {
          if (run.current !== mine) return
          setFlow({ phase: 'error', message: err instanceof Error ? err.message : 'Something went wrong.' })
        }
      }, POLL_EVERY)
    },
    [add],
  )

  const start = useCallback(async () => {
    stop()
    const mine = run.current
    setFlow({ phase: 'starting' })
    // A window opened after an await is treated as a pop-up and blocked, so it
    // is opened now, empty, and pointed at Plex once we have the link. It must
    // not be able to reach back into this page.
    const w = window.open('', '_blank', 'popup,width=520,height=720')
    if (w) w.opener = null
    popup.current = w
    try {
      const { pinId, authUrl } = await startPlex(add)
      if (run.current !== mine) return w?.close()
      if (w && !w.closed) w.location.href = authUrl
      setFlow({ phase: 'waiting', authUrl })
      poll(pinId, mine)
    } catch (err) {
      w?.close()
      if (run.current === mine) setFlow({ phase: 'error', message: err instanceof Error ? err.message : 'Something went wrong.' })
    }
  }, [add, poll, stop])

  const cancel = useCallback(() => {
    stop()
    setFlow({ phase: 'idle' })
  }, [stop])

  return { flow, start, cancel }
}
