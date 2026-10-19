import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { Breakdown, DayBars, RANGES, SessionRow, Stat } from '@/components/activity-parts'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { dailyMinutes, dataThroughAntennarr, dayLabel, formatBytes, groupByDay, spanText, summarize, totalsByChannel, totalsByWatcher } from '@/lib/activity'
import { getHistory } from '@/lib/api'
import type { HistorySession } from '@/lib/types'

/** Who watched what, and for how long. */
export function ActivityPage() {
  const { state } = useApp()
  const [days, setDays] = useState<number>(7)
  const enabled = state.settings.history
  const q = useQuery({ queryKey: ['history', days], queryFn: () => getHistory(days), enabled, refetchInterval: 30_000, staleTime: 10_000, retry: false, refetchOnWindowFocus: false })
  const sessions = useMemo(() => q.data?.sessions ?? [], [q.data])
  const now = useMemo(() => new Date(q.data?.now ?? 0), [q.data])
  const groups = useMemo(() => groupByDay(sessions), [sessions])
  const sum = useMemo(() => summarize(sessions), [sessions])
  const viewerNames = useMemo(() => new Set(state.viewers.map((v) => v.name)), [state.viewers])
  const byWatcher = useMemo(() => totalsByWatcher(sessions), [sessions])
  const byChannel = useMemo(() => totalsByChannel(sessions), [sessions])

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Activity</h1>
          <p className="mt-1 text-sm text-muted-foreground">Who watched what, and for how long. Kept for 30 days.</p>
        </div>
        {enabled && (
        <div role="group" aria-label="How far back" className="flex gap-1">
          {RANGES.map((n) => (
            <Button key={n} size="sm" variant={days === n ? 'default' : 'outline'} aria-pressed={days === n} onClick={() => setDays(n)}>
              {n} days
            </Button>
          ))}
        </div>
        )}
      </div>

      {!enabled && (
        <div className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
          The activity history is off, so nothing is being kept.{' '}
          <Link to="/settings" className="underline underline-offset-2 hover:text-foreground">
            Turn it on in Settings
          </Link>
          .
        </div>
      )}

      {enabled && !state.editable && <p className="text-xs text-muted-foreground">Antennarr has no settings file to keep this beside, so it is kept in memory and starts over when Antennarr restarts.</p>}

      {!enabled ? null : q.isPending ? (
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
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
            <Stat label="Time watched" value={spanText(sum.totalMs)} detail={`${sum.sessions} ${sum.sessions === 1 ? 'session' : 'sessions'}`} />
            <Stat label="Data sent to viewers" value={formatBytes(dataThroughAntennarr(sessions))} detail="through Antennarr, about" />
            <Stat label="Most watched channel" value={sum.topChannel?.label ?? '—'} detail={sum.topChannel ? spanText(sum.topChannel.ms) : undefined} />
            <Stat label="Most active" value={sum.topWatcher?.label ?? '—'} detail={sum.topWatcher ? spanText(sum.topWatcher.ms) : undefined} />
            <Stat label="Busiest day" value={busiest(sessions, days, now)} />
          </div>
          <DayBars sessions={sessions} days={days} now={now} />
          <div className="grid gap-4 lg:grid-cols-2">
            <Breakdown title="By viewer" rows={byWatcher} first="Data" linkTo={(label) => (viewerNames.has(label) ? `/viewers/${encodeURIComponent(label)}` : undefined)} />
            <Breakdown title="By channel" rows={byChannel} first="Reception" />
          </div>
          {groups.map((g) => (
            <section key={g.day.getTime()} aria-label={dayLabel(g.day, now)} className="space-y-2">
              <h2 className="px-1 text-sm font-semibold">{dayLabel(g.day, now)}</h2>
              <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">
                {g.sessions.map((s) => (
                  <SessionRow key={`${s.device}/${s.tuner}/${s.start}`} s={s} devices={state.devices} />
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
