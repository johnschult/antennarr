import { Globe, House } from 'lucide-react'
import { Link } from 'react-router'
import { Badge } from '@/components/ui/badge'
import { bytesOf, channelLabel, dailyBytes, dailyMinutes, formatBytes, sessionSpan, spanText, watcherOf, type Total } from '@/lib/activity'
import { deviceTitle } from '@/lib/device'
import { signalTone, toneText } from '@/lib/signal'
import type { DeviceState, HistorySession } from '@/lib/types'
import { cn } from '@/lib/utils'

// The pieces the Activity page and a viewer's page are made of.

export const RANGES = [7, 30] as const

const clock = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

export function Stat({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="rounded-xl border border-border bg-card/70 px-4 py-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate text-lg font-semibold tabular-nums">{value}</div>
      {detail && <div className="truncate text-xs text-muted-foreground">{detail}</div>}
    </div>
  )
}

const dayName = (d: Date) => d.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })

/** A bar for each day, for seeing the busy ones: the time watched, or the data sent. The list below it has the detail. */
export function DayBars({ sessions, days, now, metric = 'time' }: { sessions: HistorySession[]; days: number; now: Date; metric?: 'time' | 'data' }) {
  const totals =
    metric === 'time'
      ? dailyMinutes(sessions, days, now).map((d) => ({ day: d.day, value: d.minutes, text: d.minutes ? spanText(d.minutes * 60_000) : 'nothing' }))
      : dailyBytes(sessions, days, now).map((d) => ({ day: d.day, value: d.bytes, text: d.bytes ? formatBytes(d.bytes) : 'nothing' }))
  const most = Math.max(metric === 'time' ? 60 : 1_000_000, ...totals.map((d) => d.value))
  const title = metric === 'time' ? 'Time watched each day' : 'Data sent each day'
  return (
    <figure className="rounded-xl border border-border bg-card/70 px-4 py-3">
      <figcaption className="text-xs text-muted-foreground">{title}</figcaption>
      <div role="img" aria-label={`${title}: ${totals.map((d) => `${dayName(d.day)} ${d.text}`).join(', ')}`} className="mt-2 flex h-24 items-end gap-1">
        {totals.map((d, i) => (
          <div key={d.day.getTime()} className="flex h-full min-w-0 flex-1 flex-col justify-end" title={`${dayName(d.day)}: ${d.text}`}>
            <div
              data-testid="day-bar"
              data-value={d.value}
              data-minutes={metric === 'time' ? d.value : undefined}
              className={cn('w-full rounded-t-sm', i === totals.length - 1 ? 'bg-chart-2' : 'bg-chart-1', d.value === 0 && 'bg-muted')}
              style={{ height: d.value === 0 ? '2px' : `${Math.max(4, (d.value / most) * 100)}%` }}
            />
          </div>
        ))}
      </div>
      <div aria-hidden className="mt-1 flex justify-between text-[10px] text-muted-foreground">
        <span>{totals[0]?.day.toLocaleDateString([], { month: 'short', day: 'numeric' })}</span>
        <span>Today</span>
      </div>
    </figure>
  )
}

/** A list of totals: who or which channel, how long, and either the data they used or how good the reception was. */
export function Breakdown({
  title,
  rows,
  first,
  linkTo,
}: {
  title: string
  rows: Total[]
  first: 'Data' | 'Reception'
  /** Where a row's name goes, if anywhere. */
  linkTo?: (label: string) => string | undefined
}) {
  return (
    <section aria-label={title} className="space-y-2">
      <h2 className="px-1 text-sm font-semibold">{title}</h2>
      <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">
        {rows.slice(0, 8).map((r) => {
          const to = linkTo?.(r.label)
          return (
            <li key={r.label} data-testid="total" className="flex items-center gap-3 px-4 py-2.5 text-sm">
              <span className="min-w-0 flex-1 truncate font-medium">
                {to ? (
                  <Link to={to} className="underline-offset-2 hover:underline">
                    {r.label}
                  </Link>
                ) : (
                  r.label
                )}
              </span>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {spanText(r.ms)} · {r.sessions} {r.sessions === 1 ? 'session' : 'sessions'}
              </span>
              <span className="w-28 shrink-0 text-right text-xs tabular-nums">
                {first === 'Data' ? (
                  r.bytes > 0 ? (
                    <span title={r.through ? 'Sent through Yagi: it counts against your upload' : 'Sent straight from the tuner to your own device: it stays on your network'}>
                      {formatBytes(r.bytes)}
                      {!r.through && <span className="text-muted-foreground"> · your network</span>}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )
                ) : r.quality !== undefined ? (
                  <span className={toneText[signalTone(r.quality)]} title="The average signal quality while it was watched">
                    {r.quality}% quality
                  </span>
                ) : (
                  <span className="text-muted-foreground">—</span>
                )}
              </span>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

/** Who it was: a viewer by name, the owner's own devices, or a stream through Yagi that isn't matched to anyone. */
export function Who({ s }: { s: HistorySession }) {
  const local = s.kind === 'local'
  return (
    <Badge className={local ? 'bg-violet-500/15 text-violet-700 dark:text-violet-300' : 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300'} title={local ? s.client : undefined}>
      {local ? <House data-icon="inline-start" /> : <Globe data-icon="inline-start" />}
      {local ? 'Local' : watcherOf(s)}
    </Badge>
  )
}

/** One session: when, how long and how much data, what was on, who, and which tuner. */
export function SessionRow({ s, devices, showWho = true }: { s: HistorySession; devices: DeviceState[]; showWho?: boolean }) {
  const device = devices.find((d) => d.name === s.device)
  const several = devices.length > 1
  const tuner = `Tuner ${Number(s.tuner.replace(/\D/g, '')) + 1}`
  return (
    <li data-testid="session" className="flex flex-col gap-1.5 px-4 py-3 sm:flex-row sm:items-center sm:gap-4">
      <div className="w-40 shrink-0 text-sm tabular-nums">
        {clock(s.start)} – {s.open ? 'now' : clock(s.end)}
        <div className="text-xs text-muted-foreground">
          {sessionSpan(s)}
          {bytesOf(s) > 0 && <span className="ml-1.5">· {formatBytes(bytesOf(s))}</span>}
          {s.open && <span className="ml-1.5 rounded-full bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-700 dark:text-emerald-300">watching</span>}
        </div>
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{channelLabel(s)}</div>
        {s.titles && s.titles.length > 0 && <div className="truncate text-xs text-muted-foreground">{s.titles.join(' · ')}</div>}
      </div>
      <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
        {showWho && <Who s={s} />}
        <span>
          {several && device ? `${deviceTitle(device)} · ` : ''}
          {tuner}
        </span>
      </div>
    </li>
  )
}
