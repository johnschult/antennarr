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
