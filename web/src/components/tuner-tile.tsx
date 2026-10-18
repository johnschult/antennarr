import { Crown, Globe, House, Tv } from 'lucide-react'
import { ChannelLogo } from '@/components/channel-logo'
import { SignalMeter } from '@/components/signal-meter'
import { Sparkline } from '@/components/sparkline'
import { Badge } from '@/components/ui/badge'
import { useHistory } from '@/hooks/use-history'
import { formatMbps } from '@/lib/format'
import type { GuideEntry } from '@/lib/types'
import { ownerLabel, type Tile } from '@/lib/slots'
import { cn } from '@/lib/utils'

const badgeClass: Record<Tile['owner']['kind'], string> = {
  viewer: 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300',
  remote: 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-300',
  local: 'bg-violet-500/15 text-violet-700 dark:text-violet-300',
  reserved: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  free: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
}

/** The last couple of minutes of this stream's bitrate. Mounted only while a stream is live, so each new stream starts with a blank line. */
function StreamSparkline({ bps, tick }: { bps?: number; tick: unknown }) {
  const history = useHistory(bps === undefined ? undefined : bps / 1_000_000, tick)
  return <Sparkline data={history} height={28} label="Bitrate" />
}

/** Where a stream is going, for the footer; the pill already says who it is. */
function where({ slot, owner, stream }: Tile): string {
  if (owner.kind === 'local') return slot?.targetIP ?? 'Local device'
  return stream?.client.replace(/:\d+$/, '') ?? ''
}

interface TileProps {
  tile: Tile
  dimmed?: boolean
  /** Changes with each state update, so the bitrate line gets a sample each time. */
  tick?: unknown
  /** What the guide says about this channel, if anything. */
  guide?: GuideEntry
  /** Where this channel's logo is served from. */
  iconSrc?: string
}

/** One physical tuner: what's on it and who has it. */
export function TunerTile({ tile, dimmed = false, tick, guide, iconSrc }: TileProps) {
  const { slot, owner } = tile
  const live = slot?.inUse === true

  return (
    <div
      data-testid="tuner-tile"
      data-owner={owner.kind}
      className={cn(
        'relative flex h-[18rem] flex-col overflow-hidden rounded-2xl border p-5',
        live ? 'border-border bg-card/70' : 'border-dashed border-border bg-muted/30',
        dimmed && 'opacity-50',
      )}
    >
      {live && (
        <div className="pointer-events-none absolute inset-0 text-foreground opacity-[0.06] [background:repeating-linear-gradient(0deg,currentColor_0px,currentColor_1px,transparent_1px,transparent_3px)]" />
      )}
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-muted-foreground">TUNER {tile.index + 1}</span>
        <Badge className={badgeClass[owner.kind]}>
          {owner.kind === 'reserved' && <Crown data-icon="inline-start" />}
          {owner.kind === 'local' && <House data-icon="inline-start" />}
          {(owner.kind === 'viewer' || owner.kind === 'remote') && <Globe data-icon="inline-start" />}
          {ownerLabel(owner)}
        </Badge>
      </div>

      {live ? (
        <>
          {/* pt-3: however little room is left, the logo never touches the pill above it */}
          <div className="mt-auto pt-3">
            <div className="flex items-end justify-between gap-3">
              <div className="text-5xl font-bold tracking-tight tabular-nums">{slot.channel ?? '—'}</div>
              {guide?.icon && iconSrc && <ChannelLogo key={iconSrc} src={iconSrc} name={slot.channelName ?? guide.name ?? 'Channel'} className="h-10 max-w-24" />}
            </div>
            <div className="text-lg text-foreground">{slot.channelName ?? 'Tuning…'}</div>
            {guide?.now && (
              <div data-testid="tile-now" className="truncate text-sm text-muted-foreground" title={guide.now.subTitle ? `${guide.now.title}: ${guide.now.subTitle}` : guide.now.title}>
                {guide.now.title}
              </div>
            )}
          </div>
          <div className="mt-2" data-testid="tile-bitrate">
            {/* keyed so a different channel or a different viewer is a new stream with a new line */}
            <StreamSparkline key={`${slot.channel}|${slot.targetIP}|${owner.kind}`} bps={slot.bitsPerSecond} tick={tick} />
          </div>
          {slot.signalStrength !== undefined && (
            <div className="mt-2" data-testid="tile-signal">
              <SignalMeter strength={slot.signalStrength} quality={slot.signalQuality} />
            </div>
          )}
          <div className="mt-2 flex items-center justify-between border-t border-border pt-3 text-xs text-muted-foreground">
            <span className="truncate">{where(tile)}</span>
            <span className="tabular-nums">{formatMbps(slot.bitsPerSecond)}</span>
          </div>
        </>
      ) : (
        <div className="m-auto text-center text-sm text-muted-foreground">
          {owner.kind === 'reserved' && <Tv className="mx-auto mb-2 size-6" />}
          {owner.kind === 'reserved' ? 'Held for your TVs' : 'Idle'}
        </div>
      )}
    </div>
  )
}
