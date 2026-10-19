/** A reading from the tuner, kept within 0 to 100. */
export const clampPct = (n: number) => Math.min(100, Math.max(0, Math.round(n)))

export type SignalTone = 'good' | 'fair' | 'poor'

/** Good when the reception is good, fair when it is marginal, poor when it is poor. */
export function signalTone(pct: number): SignalTone {
  return pct >= 80 ? 'good' : pct >= 50 ? 'fair' : 'poor'
}

/** What to color a reading's text: nothing special when it is good, so only trouble stands out. */
export const toneText: Record<SignalTone, string> = {
  good: '',
  fair: 'text-amber-700 dark:text-amber-300',
  poor: 'text-red-700 dark:text-red-300',
}

/** How many bars a strength is drawn with: four when it is strong, none when there is next to nothing. */
export function signalBars(pct: number): 0 | 1 | 2 | 3 | 4 {
  return pct >= 80 ? 4 : pct >= 60 ? 3 : pct >= 40 ? 2 : pct >= 20 ? 1 : 0
}
