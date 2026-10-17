/** 14_200_000 -> "14.2 Mbps". */
export function formatMbps(bitsPerSecond: number | undefined): string {
  if (!bitsPerSecond) return '0 Mbps'
  const m = bitsPerSecond / 1_000_000
  return `${m >= 10 ? m.toFixed(0) : m.toFixed(1)} Mbps`
}

/** "just now", "4m ago", "3h ago", "2d ago". */
export function timeAgo(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return 'never'
  const secs = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 1000))
  if (secs < 10) return 'just now'
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

/** "1h 05m", "7m 03s" — how long a stream has been running. */
export function duration(iso: string, now: Date = new Date()): string {
  const secs = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 1000))
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  const s = secs % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}h ${pad(m)}m` : `${m}m ${pad(s)}s`
}

/** One address per line or comma separated -> clean list. */
export function parseAddresses(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}
