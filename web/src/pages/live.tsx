import { TunerTile } from '@/components/tuner-tile'
import { Sparkline } from '@/components/sparkline'
import { Badge } from '@/components/ui/badge'
import { useApp } from '@/hooks/app-context'
import { useHistory } from '@/hooks/use-history'
import { formatMbps, timeAgo } from '@/lib/format'
import { buildTiles } from '@/lib/slots'
import type { EventKind, Viewer } from '@/lib/types'
import { cn } from '@/lib/utils'

const dot: Record<EventKind, string> = {
  ok: 'bg-emerald-400',
  info: 'bg-cyan-400',
  warn: 'bg-amber-400',
  err: 'bg-red-400',
}

export function LivePage() {
  const { state } = useApp()
  const { summary, tuner, viewers } = state
  const tiles = buildTiles(state)

  const remoteBps = tuner.slots.filter((s) => s.inUse && s.viaAntennarr).reduce((a, s) => a + (s.bitsPerSecond ?? 0), 0)
  const totalBps = tuner.slots.reduce((a, s) => a + (s.bitsPerSecond ?? 0), 0)
  const history = useHistory(remoteBps / 1_000_000, state.time)

  const parts = [
    `${summary.remote} remote`,
    `${summary.local} local`,
    `${summary.reserved} held for you`,
    formatMbps(totalBps),
  ]

  return (
    <div className="space-y-8">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div>
          <div className="text-xs tracking-[0.2em] text-cyan-300/80 uppercase">On air now</div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">
            {summary.inUse} of {summary.total} tuners live
          </h1>
          <p className="mt-1 text-zinc-400">{parts.join(' · ')}</p>
        </div>
        <div className="w-full sm:w-72">
          <div className="mb-1 flex justify-between text-xs text-zinc-500">
            <span>upload to viewers</span>
            <span className="tabular-nums">{formatMbps(remoteBps)}</span>
          </div>
          <Sparkline data={history} height={56} />
        </div>
      </div>

      <section aria-label="Tuners" className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {tiles.map((t) => (
          <TunerTile key={t.index} tile={t} dimmed={!tuner.online} />
        ))}
      </section>

      <div className="grid gap-6 lg:grid-cols-2">
        <section>
          <h2 className="mb-3 text-sm font-semibold">Viewers</h2>
          {viewers.length === 0 ? (
            <p className="rounded-xl border border-dashed border-zinc-800 px-4 py-6 text-sm text-zinc-500">
              No viewers yet. Anyone who can reach this port may use the tuner.
            </p>
          ) : (
            <div className="space-y-2">
              {viewers.map((v) => (
                <ViewerRow key={v.name} v={v} />
              ))}
            </div>
          )}
        </section>

        <section>
          <h2 className="mb-3 text-sm font-semibold">Timeline</h2>
          {tuner.events.length === 0 ? (
            <p className="text-sm text-zinc-500">Nothing has happened yet.</p>
          ) : (
            <ol className="relative ml-2 space-y-5 border-l border-zinc-800">
              {[...tuner.events].reverse().map((e, i) => (
                <li key={`${e.time}-${i}`} className="relative pl-5">
                  <span className={cn('absolute top-1.5 -left-[5px] size-2.5 rounded-full ring-4 ring-zinc-950', dot[e.kind])} />
                  <div className="text-sm">{e.text}</div>
                  <div className="text-xs text-zinc-500">{timeAgo(e.time)}</div>
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </div>
  )
}

function ViewerRow({ v }: { v: Viewer }) {
  return (
    <div className="flex items-center gap-4 rounded-xl border border-zinc-800 bg-zinc-900/60 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 font-medium">
          {v.name}
          {v.inUse > 0 && <Badge className="bg-emerald-500/15 text-emerald-300">Watching</Badge>}
        </div>
        <div className="truncate text-xs text-zinc-500">
          {v.addresses.join(', ')} · {v.lastSeen ? `seen ${timeAgo(v.lastSeen)}` : 'waiting for first contact'}
        </div>
      </div>
      <div className="flex gap-1" role="img" aria-label={`${v.inUse} of ${v.limit} tuners in use`}>
        {Array.from({ length: v.limit }).map((_, i) => (
          <div key={i} className={cn('h-6 w-3 rounded-sm', i < v.inUse ? 'bg-cyan-400' : 'bg-zinc-800')} />
        ))}
      </div>
    </div>
  )
}
