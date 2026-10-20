import { useCallback, useEffect, useState } from 'react'
import { discoverTuners } from '@/lib/api'
import type { FoundTuner } from '@/lib/types'

/** Look every few seconds for a few minutes, then stop until the person asks again. */
export const LOOK_EVERY_MS = 5_000
export const LOOK_FOR_MS = 3 * 60_000

/**
 * Looks for tuners on the network while the page is open, every few seconds for a few minutes, then stops, so a
 * machine that will never have a tuner doesn't broadcast for ever. `lookAgain` starts another round.
 */
export function useFindTuners() {
  const [found, setFound] = useState<FoundTuner[]>([])
  const [looking, setLooking] = useState(true)
  const [round, setRound] = useState(0)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!looking) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const until = Date.now() + LOOK_FOR_MS
    const once = async () => {
      try {
        const list = await discoverTuners()
        if (cancelled) return
        setFound(list)
        setError(null)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'The search failed.')
      }
      if (cancelled) return
      if (Date.now() >= until) setLooking(false)
      else timer = setTimeout(() => void once(), LOOK_EVERY_MS)
    }
    void once()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [looking, round])

  const lookAgain = useCallback(() => {
    setRound((r) => r + 1)
    setLooking(true)
  }, [])

  return { found, looking, error, lookAgain }
}
