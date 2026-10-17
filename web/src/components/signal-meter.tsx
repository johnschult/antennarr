import { cn } from '@/lib/utils'

const clamp = (n: number) => Math.min(100, Math.max(0, Math.round(n)))

/** Green when the reception is good, amber when it is marginal, red when it is poor. */
export function signalTone(pct: number): 'good' | 'fair' | 'poor' {
  return pct >= 80 ? 'good' : pct >= 50 ? 'fair' : 'poor'
}

const fill = { good: 'bg-emerald-500', fair: 'bg-amber-500', poor: 'bg-red-500' }

/** One row of the meter. It sits in the parent's grid, so labels, bars and percentages line up between rows. */
function Meter({ label, pct }: { label: string; pct: number }) {
  const v = clamp(pct)
  return (
    <div className="contents" data-testid={`signal-${label.toLowerCase()}`} data-tone={signalTone(v)}>
      <span>{label}</span>
      <span className="h-1 overflow-hidden rounded-full bg-muted" aria-hidden="true">
        <span className={cn('block h-full rounded-full', fill[signalTone(v)])} style={{ width: `${v}%` }} />
      </span>
      <span className="text-right tabular-nums">{v}%</span>
    </div>
  )
}

/** How well the tuner is receiving the channel: signal strength and quality, as percentages. */
export function SignalMeter({ strength, quality }: { strength: number; quality?: number }) {
  return (
    <div
      role="group"
      aria-label={`Signal strength ${clamp(strength)}%${quality === undefined ? '' : `, quality ${clamp(quality)}%`}`}
      className="grid grid-cols-[auto_1fr_auto] items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
    >
      <Meter label="Signal" pct={strength} />
      {quality !== undefined && <Meter label="Quality" pct={quality} />}
    </div>
  )
}
