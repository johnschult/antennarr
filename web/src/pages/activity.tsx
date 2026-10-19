import { useQuery } from '@tanstack/react-query'
import { Globe, House, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { channelLabel, dailyMinutes, dayLabel, groupByDay, sessionSpan, spanText, summarize, watcherOf } from '@/lib/activity'
import { getHistory } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import type { DeviceState, HistorySession } from '@/lib/types'
import { cn } from '@/lib/utils'

const RANGES = [7, 30] as const

const clock = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

function Stat({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="rounded-xl border border-border bg-card/70 px-4 py-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate text-lg font-semibold tabular-nums">{value}</div>
      {detail && <div className="truncate text-xs text-muted-foreground">{detail}</div>}
    </div>
  )
}

/** Hours watched each day, as bars. The list below it has the detail; this is for seeing the busy days. */
function DayBars({ sessions, days, now }: { sessions: HistorySession[]; days: number; now: Date }) {
  const totals = dailyMinutes(sessions, days, now)
  const most = Math.max(60, ...totals.map((d) => d.minutes))
  const name = (d: Date) => d.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
  return (
    <figure className="rounded-xl border border-border bg-card/70 px-4 py-3">
      <figcaption className="text-xs text-muted-foreground">Time watched each day</figcaption>
      <div
        role="img"
        aria-label={`Time watched each day: ${totals.map((d) => `${name(d.day)} ${spanText(d.minutes * 60_000)}`).join(', ')}`}
        className="mt-2 flex h-24 items-end gap-1"
      >
        {totals.map((d, i) => (
          <div key={d.day.getTime()} className="flex h-full min-w-0 flex-1 flex-col justify-end" title={`${name(d.day)}: ${d.minutes ? spanText(d.minutes * 60_000) : 'nothing'}`}>
            <div
              data-testid="day-bar"
              data-minutes={d.minutes}
              className={cn('w-full rounded-t-sm', i === totals.length - 1 ? 'bg-chart-2' : 'bg-chart-1', d.minutes === 0 && 'bg-muted')}
              style={{ height: d.minutes === 0 ? '2px' : `${Math.max(4, (d.minutes / most) * 100)}%` }}
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

/** Who it was: a viewer by name, the owner's own devices, or a stream through Antennarr that isn't matched to anyone. */
function Who({ s }: { s: HistorySession }) {
  const local = s.kind === 'local'
  return (
    <Badge className={local ? 'bg-violet-500/15 text-violet-700 dark:text-violet-300' : 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300'} title={local ? s.client : undefined}>
      {local ? <House data-icon="inline-start" /> : <Globe data-icon="inline-start" />}
      {local ? 'Local' : watcherOf(s)}
    </Badge>
  )
}

function Row({ s, devices }: { s: HistorySession; devices: DeviceState[] }) {
  const device = devices.find((d) => d.name === s.device)
  const several = devices.length > 1
  const tuner = `Tuner ${Number(s.tuner.replace(/\D/g, '')) + 1}`
  return (
    <li data-testid="session" className="flex flex-col gap-1.5 px-4 py-3 sm:flex-row sm:items-center sm:gap-4">
      <div className="w-40 shrink-0 text-sm tabular-nums">
        {clock(s.start)} – {s.open ? 'now' : clock(s.end)}
        <div className="text-xs text-muted-foreground">
          {sessionSpan(s)}
          {s.open && <span className="ml-1.5 rounded-full bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-700 dark:text-emerald-300">watching</span>}
        </div>
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{channelLabel(s)}</div>
        {s.titles && s.titles.length > 0 && <div className="truncate text-xs text-muted-foreground">{s.titles.join(' · ')}</div>}
      </div>
      <div className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
        <Who s={s} />
        <span>
          {several && device ? `${deviceTitle(device)} · ` : ''}
          {tuner}
        </span>
      </div>
    </li>
  )
}

/** Who watched what, and for how long. */
export function ActivityPage() {
  const { state } = useApp()
  const [days, setDays] = useState<number>(7)
  const q = useQuery({ queryKey: ['history', days], queryFn: () => getHistory(days), refetchInterval: 30_000, staleTime: 10_000, retry: false, refetchOnWindowFocus: false })
  const sessions = useMemo(() => q.data?.sessions ?? [], [q.data])
  const now = useMemo(() => new Date(q.data?.now ?? 0), [q.data])
  const groups = useMemo(() => groupByDay(sessions), [sessions])
  const sum = useMemo(() => summarize(sessions), [sessions])

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Activity</h1>
          <p className="mt-1 text-sm text-muted-foreground">Who watched what, and for how long. Kept for 30 days.</p>
        </div>
        <div role="group" aria-label="How far back" className="flex gap-1">
          {RANGES.map((n) => (
            <Button key={n} size="sm" variant={days === n ? 'default' : 'outline'} aria-pressed={days === n} onClick={() => setDays(n)}>
              {n} days
            </Button>
          ))}
        </div>
      </div>

      {!state.editable && <p className="text-xs text-muted-foreground">Antennarr has no settings file to keep this beside, so it is kept in memory and starts over when Antennarr restarts.</p>}

      {q.isPending ? (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 aria-hidden className="size-4 animate-spin" /> Loading…
        </p>
      ) : q.isError ? (
        <p role="alert" className="text-sm text-red-700 dark:text-red-300">
          {q.error instanceof Error ? q.error.message : "Couldn't load the activity."}
        </p>
      ) : sessions.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
          Nothing watched in the last {days} days. Once someone watches, it appears here.
        </div>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Stat label="Time watched" value={spanText(sum.totalMs)} detail={`${sum.sessions} ${sum.sessions === 1 ? 'session' : 'sessions'}`} />
            <Stat label="Most watched channel" value={sum.topChannel?.label ?? '—'} detail={sum.topChannel ? spanText(sum.topChannel.ms) : undefined} />
            <Stat label="Most active" value={sum.topWatcher?.label ?? '—'} detail={sum.topWatcher ? spanText(sum.topWatcher.ms) : undefined} />
            <Stat label="Busiest day" value={busiest(sessions, days, now)} />
          </div>
          <DayBars sessions={sessions} days={days} now={now} />
          {groups.map((g) => (
            <section key={g.day.getTime()} aria-label={dayLabel(g.day, now)} className="space-y-2">
              <h2 className="px-1 text-sm font-semibold">{dayLabel(g.day, now)}</h2>
              <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">
                {g.sessions.map((s) => (
                  <Row key={`${s.device}/${s.tuner}/${s.start}`} s={s} devices={state.devices} />
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
    </div>
  )
}

/** The day with the most watched, by its name and how much. */
function busiest(sessions: HistorySession[], days: number, now: Date): string {
  const top = dailyMinutes(sessions, days, now).reduce((a, b) => (b.minutes > a.minutes ? b : a))
  if (top.minutes === 0) return '—'
  return `${top.day.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })} · ${spanText(top.minutes * 60_000)}`
}
