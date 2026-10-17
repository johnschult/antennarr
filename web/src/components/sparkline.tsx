import { useId } from 'react'

interface Props {
  data: number[]
  height?: number
  /** Top of the scale; defaults to the largest sample (min 1). */
  max?: number
  label?: string
}

export function Sparkline({ data, height = 48, max, label = 'Recent upload' }: Props) {
  const gradient = `spark-${useId().replace(/[^a-zA-Z0-9_-]/g, '')}` // one per chart, so several can share a page
  const w = 300
  const top = max ?? Math.max(1, ...data)
  if (data.length < 2) {
    return <div role="img" aria-label={`${label}: no data yet`} style={{ height }} className="w-full" />
  }
  const pts = data.map((v, i) => [(i / (data.length - 1)) * w, height - (v / top) * (height - 4) - 2] as const)
  const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)} ${y.toFixed(1)}`).join(' ')
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className="w-full text-cyan-600 dark:text-cyan-400" style={{ height }}>
      <defs>
        <linearGradient id={gradient} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="currentColor" stopOpacity=".35" />
          <stop offset="1" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={`${line} L${w} ${height} L0 ${height} Z`} fill={`url(#${gradient})`} />
      <path d={line} stroke="currentColor" strokeWidth="1.5" fill="none" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
