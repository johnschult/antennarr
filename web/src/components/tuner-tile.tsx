import { Antenna, Shield } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { formatMbps } from '@/lib/format'
import { ownerLabel, type Tile } from '@/lib/slots'
import { cn } from '@/lib/utils'

const badgeClass: Record<Tile['owner']['kind'], string> = {
  viewer: 'bg-cyan-500/15 text-cyan-300',
  remote: 'bg-cyan-500/15 text-cyan-300',
  local: 'bg-violet-500/15 text-violet-300',
  reserved: 'bg-zinc-500/15 text-zinc-300',
  free: 'bg-emerald-500/15 text-emerald-300',
}

/** One physical tuner: what's on it and who has it. */
export function TunerTile({ tile, dimmed = false }: { tile: Tile; dimmed?: boolean }) {
  const { slot, owner } = tile
  const live = slot?.inUse === true

  return (
    <div
      data-testid="tuner-tile"
      data-owner={owner.kind}
      className={cn(
        'relative flex h-64 flex-col overflow-hidden rounded-2xl border p-5',
        live ? 'border-zinc-700 bg-zinc-900/80' : 'border-dashed border-zinc-800 bg-zinc-950/40',
        dimmed && 'opacity-50',
      )}
    >
      {live && (
        <div className="pointer-events-none absolute inset-0 opacity-[0.07] [background:repeating-linear-gradient(0deg,#fff_0px,#fff_1px,transparent_1px,transparent_3px)]" />
      )}
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-zinc-500">TUNER {tile.index + 1}</span>
        <Badge className={badgeClass[owner.kind]}>{ownerLabel(owner)}</Badge>
      </div>

      {live ? (
        <>
          <div className="mt-auto">
            <div className="text-5xl font-bold tracking-tight tabular-nums">{slot.channel ?? '—'}</div>
            <div className="text-lg text-zinc-300">{slot.channelName ?? 'Tuning…'}</div>
          </div>
          <div className="mt-4 flex items-center justify-between border-t border-zinc-800 pt-3 text-xs text-zinc-400">
            <span className="truncate">{owner.kind === 'local' ? (slot.targetIP ?? 'Local device') : ownerLabel(owner)}</span>
            <span className="tabular-nums">{formatMbps(slot.bitsPerSecond)}</span>
          </div>
        </>
      ) : (
        <div className="m-auto text-center text-sm text-zinc-500">
          {owner.kind === 'reserved' ? (
            <>
              <Shield className="mx-auto mb-2 size-6" />
              Held for your TVs
            </>
          ) : (
            <>
              <Antenna className="mx-auto mb-2 size-6" />
              Ready
            </>
          )}
        </div>
      )}
    </div>
  )
}
