import type { HistorySession } from './types'

const MIN = 60_000

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate())
const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n)

/** Whole minutes a session ran (at least one: anything kept lasted at least fifteen seconds). */
export function minutesOf(s: HistorySession): number {
  return Math.max(1, Math.round((new Date(s.end).getTime() - new Date(s.start).getTime()) / MIN))
}

/** "1h 35m", "12m", "45s": how long, for people. */
export function spanText(ms: number): string {
  if (ms < MIN) return `${Math.max(1, Math.round(ms / 1000))}s`
  const mins = Math.round(ms / MIN)
  const h = Math.floor(mins / 60)
  return h > 0 ? `${h}h ${String(mins % 60).padStart(2, '0')}m` : `${mins}m`
}

/** How long a session ran, for people. */
export function sessionSpan(s: HistorySession): string {
  return spanText(new Date(s.end).getTime() - new Date(s.start).getTime())
}

/** "Today", "Yesterday", or "Mon, Oct 12": what a day is called in the list. */
export function dayLabel(day: Date, now: Date): string {
  const today = startOfDay(now)
  if (day.getTime() === today.getTime()) return 'Today'
  if (day.getTime() === addDays(today, -1).getTime()) return 'Yesterday'
  return day.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
}

export interface DayGroup {
  day: Date
  sessions: HistorySession[]
}

/** Sessions by the day they began, newest day first and newest session first within it. */
export function groupByDay(sessions: HistorySession[]): DayGroup[] {
  const groups = new Map<number, DayGroup>()
  for (const s of [...sessions].sort((a, b) => new Date(b.start).getTime() - new Date(a.start).getTime())) {
    const day = startOfDay(new Date(s.start))
    const g = groups.get(day.getTime()) ?? { day, sessions: [] }
    g.sessions.push(s)
    groups.set(day.getTime(), g)
  }
  return [...groups.values()].sort((a, b) => b.day.getTime() - a.day.getTime())
}

export interface DayTotal {
  day: Date
  minutes: number
}

/** Minutes watched on each of the last `days` days, oldest first, ending with today. A session that runs past midnight counts on both days. */
export function dailyMinutes(sessions: HistorySession[], days: number, now: Date): DayTotal[] {
  const today = startOfDay(now)
  const out: DayTotal[] = []
  for (let i = days - 1; i >= 0; i--) out.push({ day: addDays(today, -i), minutes: 0 })
  for (const s of sessions) {
    const start = new Date(s.start).getTime()
    const end = new Date(s.end).getTime()
    for (const d of out) {
      const from = d.day.getTime()
      const to = addDays(d.day, 1).getTime()
      const overlap = Math.min(end, to) - Math.max(start, from)
      if (overlap > 0) d.minutes += overlap / MIN
    }
  }
  return out.map((d) => ({ ...d, minutes: Math.round(d.minutes) }))
}

export interface Summary {
  totalMs: number
  sessions: number
  topChannel?: { label: string; ms: number }
  topWatcher?: { label: string; ms: number }
}

/** Who a session was, for counting: a viewer's name, "Remote", or "Your devices". */
export function watcherOf(s: HistorySession): string {
  return s.kind === 'viewer' ? (s.viewer ?? 'Remote') : s.kind === 'local' ? 'Your devices' : 'Remote'
}

/** A channel as people know it: its number and name. */
export const channelLabel = (s: HistorySession) => (s.channelName ? `${s.channel} ${s.channelName}` : s.channel)

/** The totals at the top of the page: how much was watched, what the most was, and by whom. */
export function summarize(sessions: HistorySession[]): Summary {
  const channels = new Map<string, number>()
  const watchers = new Map<string, number>()
  let totalMs = 0
  for (const s of sessions) {
    const ms = Math.max(0, new Date(s.end).getTime() - new Date(s.start).getTime())
    totalMs += ms
    channels.set(channelLabel(s), (channels.get(channelLabel(s)) ?? 0) + ms)
    watchers.set(watcherOf(s), (watchers.get(watcherOf(s)) ?? 0) + ms)
  }
  const top = (m: Map<string, number>) => {
    let best: { label: string; ms: number } | undefined
    for (const [label, ms] of m) if (!best || ms > best.ms) best = { label, ms }
    return best
  }
  return { totalMs, sessions: sessions.length, topChannel: top(channels), topWatcher: top(watchers) }
}

/** "1.4 GB", "230 MB", "12 MB", "800 KB": how much data, for people (decimal units, as network figures are). */
export function formatBytes(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(n >= 1e10 ? 0 : 1)} GB`
  if (n >= 1e6) return `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1)} MB`
  if (n >= 1e3) return `${Math.round(n / 1e3)} KB`
  return `${Math.round(n)} B`
}

/** A session's data, if it was measured. */
export const bytesOf = (s: HistorySession) => Math.max(0, s.bytes ?? 0)

/** Whether a stream went out through Antennarr (a viewer's), rather than straight from the tuner to the owner's own device. */
export const throughAntennarr = (s: HistorySession) => s.kind !== 'local'

export interface Total {
  label: string
  /** Time watched, in milliseconds. */
  ms: number
  bytes: number
  sessions: number
  /** The average signal quality over the time it was reported, 0 to 100, when it was. */
  quality?: number
  /** Whether it went through Antennarr: the data counts against the sharer's upload. */
  through: boolean
}

function totalsBy(sessions: HistorySession[], key: (s: HistorySession) => string): Total[] {
  const rows = new Map<string, Total & { qMs: number; qSum: number }>()
  for (const s of sessions) {
    const label = key(s)
    const ms = Math.max(0, new Date(s.end).getTime() - new Date(s.start).getTime())
    const r = rows.get(label) ?? { label, ms: 0, bytes: 0, sessions: 0, through: throughAntennarr(s), qMs: 0, qSum: 0 }
    r.ms += ms
    r.bytes += bytesOf(s)
    r.sessions++
    if (s.quality) {
      r.qMs += ms
      r.qSum += s.quality * ms
    }
    rows.set(label, r)
  }
  return [...rows.values()]
    .map(({ qMs, qSum, ...t }) => ({ ...t, quality: qMs > 0 ? Math.round(qSum / qMs) : undefined }))
    .sort((a, b) => b.bytes - a.bytes || b.ms - a.ms)
}

/** Who watched, with how long and how much data each: most data first. */
export const totalsByWatcher = (sessions: HistorySession[]) => totalsBy(sessions, watcherOf)

/** Which channels were watched, with how long, how much data and how good the reception was: most time first. */
export const totalsByChannel = (sessions: HistorySession[]) => totalsBy(sessions, channelLabel).sort((a, b) => b.ms - a.ms)

/** The data that went out through Antennarr (viewers and unmatched streams), which is what the sharer's upload paid for. */
export const dataThroughAntennarr = (sessions: HistorySession[]) => sessions.filter(throughAntennarr).reduce((n, s) => n + bytesOf(s), 0)

export interface DayBytes {
  day: Date
  bytes: number
}

/** Bytes sent each of the last `days` days, oldest first, ending today. A session that runs past midnight is split between the days by how long it was on each. */
export function dailyBytes(sessions: HistorySession[], days: number, now: Date): DayBytes[] {
  const today = startOfDay(now)
  const out: DayBytes[] = []
  for (let i = days - 1; i >= 0; i--) out.push({ day: addDays(today, -i), bytes: 0 })
  for (const s of sessions) {
    const b = bytesOf(s)
    const start = new Date(s.start).getTime()
    const end = new Date(s.end).getTime()
    if (b === 0 || !(end > start)) continue
    for (const d of out) {
      const overlap = Math.min(end, addDays(d.day, 1).getTime()) - Math.max(start, d.day.getTime())
      if (overlap > 0) d.bytes += (b * overlap) / (end - start)
    }
  }
  return out.map((d) => ({ ...d, bytes: Math.round(d.bytes) }))
}

/**
 * How good the reception has been on each channel of one tuner, from the sessions kept: the average signal
 * quality while it was watched, weighted by how long, by channel number. Only a channel that was watched,
 * with the tuner reporting a quality, has an entry.
 */
export function receptionByChannel(sessions: HistorySession[], device: string): Map<string, number> {
  const sums = new Map<string, { ms: number; sum: number }>()
  for (const s of sessions) {
    if (s.device !== device || !s.quality) continue
    const ms = Math.max(0, new Date(s.end).getTime() - new Date(s.start).getTime())
    const e = sums.get(s.channel) ?? { ms: 0, sum: 0 }
    e.ms += ms
    e.sum += s.quality * ms
    sums.set(s.channel, e)
  }
  const out = new Map<string, number>()
  for (const [channel, { ms, sum }] of sums) if (ms > 0) out.set(channel, Math.round(sum / ms))
  return out
}
