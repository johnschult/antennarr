import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router'
import { Breakdown, DayBars, RANGES, SessionRow, Stat } from '@/components/activity-parts'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { dailyBytes, dayLabel, formatBytes, groupByDay, spanText, summarize, totalsByChannel } from '@/lib/activity'
import { getHistory } from '@/lib/api'

/** One viewer: how much data they used and how long they watched, each day, what they watched, and their sessions. */
export function ViewerPage() {
  const { name = '' } = useParams()
  const { state } = useApp()
  const viewer = state.viewers.find((v) => v.name === name)
  const [days, setDays] = useState<number>(30)
  const enabled = state.settings.history && viewer !== undefined
  const q = useQuery({ queryKey: ['history', days], queryFn: () => getHistory(days), enabled, refetchInterval: 30_000, staleTime: 10_000, retry: false, refetchOnWindowFocus: false })
  const mine = useMemo(() => (q.data?.sessions ?? []).filter((s) => s.kind === 'viewer' && s.viewer === name), [q.data, name])
  const now = useMemo(() => new Date(q.data?.now ?? 0), [q.data])
  const groups = useMemo(() => groupByDay(mine), [mine])
  const sum = useMemo(() => summarize(mine), [mine])
  const channels = useMemo(() => totalsByChannel(mine), [mine])
  const total = useMemo(() => mine.reduce((n, s) => n + (s.bytes ?? 0), 0), [mine])
  const heaviest = useMemo(() => dailyBytes(mine, days, now).reduce((a, b) => (b.bytes > a.bytes ? b : a), { day: now, bytes: 0 }), [mine, days, now])

  const back = (
    <Link to="/viewers" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground underline-offset-2 hover:text-foreground hover:underline">
      <ArrowLeft aria-hidden className="size-4" /> Viewers
    </Link>
  )

  if (!viewer) {
    return (
      <div className="space-y-4">
        {back}
        <h1 className="text-xl font-semibold tracking-tight">No viewer called {name}</h1>
        <p className="text-sm text-muted-foreground">It may have been removed. The viewers there are now are on the Viewers page.</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {back}
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{viewer.name}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {viewer.addresses.join(', ')} · what they watched and how much data it was, for the last {days} days.
          </p>
        </div>
        {state.settings.history && (
          <div role="group" aria-label="How far back" className="flex gap-1">
            {RANGES.map((n) => (
              <Button key={n} size="sm" variant={days === n ? 'default' : 'outline'} aria-pressed={days === n} onClick={() => setDays(n)}>
                {n} days
              </Button>
            ))}
          </div>
        )}
      </div>

      {!state.settings.history ? (
        <div className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
          The activity history is off, so nothing is being kept.{' '}
          <Link to="/settings" className="underline underline-offset-2 hover:text-foreground">
            Turn it on in Settings
          </Link>
          .
        </div>
      ) : q.isPending ? (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 aria-hidden className="size-4 animate-spin" /> Loading…
        </p>
      ) : q.isError ? (
        <p role="alert" className="text-sm text-red-700 dark:text-red-300">
          {q.error instanceof Error ? q.error.message : "Couldn't load the activity."}
        </p>
      ) : mine.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
          {viewer.name} hasn't watched anything in the last {days} days.
        </div>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Stat label="Data used" value={formatBytes(total)} detail="through Antennarr, about" />
            <Stat label="Time watched" value={spanText(sum.totalMs)} detail={`${sum.sessions} ${sum.sessions === 1 ? 'session' : 'sessions'}`} />
            <Stat label="Most data in a day" value={heaviest.bytes > 0 ? formatBytes(heaviest.bytes) : '—'} detail={heaviest.bytes > 0 ? heaviest.day.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' }) : undefined} />
            <Stat label="Most watched channel" value={sum.topChannel?.label ?? '—'} detail={sum.topChannel ? spanText(sum.topChannel.ms) : undefined} />
          </div>
          <DayBars sessions={mine} days={days} now={now} metric="data" />
          <DayBars sessions={mine} days={days} now={now} metric="time" />
          <Breakdown title="What they watched" rows={channels} first="Reception" />
          {groups.map((g) => (
            <section key={g.day.getTime()} aria-label={dayLabel(g.day, now)} className="space-y-2">
              <h2 className="px-1 text-sm font-semibold">{dayLabel(g.day, now)}</h2>
              <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">
                {g.sessions.map((s) => (
                  <SessionRow key={`${s.device}/${s.tuner}/${s.start}`} s={s} devices={state.devices} showWho={false} />
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
    </div>
  )
}
