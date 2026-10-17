import { useEffect, useState } from 'react'

/**
 * Keeps the last `max` samples of a number. A sample is taken whenever `tick`
 * changes (e.g. the timestamp of each state update), even if the value is the
 * same, so a flat line still has the right length.
 */
export function useHistory(value: number | undefined, tick: unknown, max = 90): number[] {
  const [history, setHistory] = useState<number[]>([])

  useEffect(() => {
    if (value === undefined) return
    setHistory((h) => [...h.slice(-(max - 1)), value])
    // eslint-disable-next-line react-hooks/exhaustive-deps -- sample per tick, not per value
  }, [tick, max])

  return history
}
