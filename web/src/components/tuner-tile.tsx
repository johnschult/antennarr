import { Crown, Globe, House, Network, Signal, Tv } from 'lucide-react'
import { useState } from 'react'
import { ChannelLogo } from '@/components/channel-logo'
import { Sparkline } from '@/components/sparkline'
import { Badge } from '@/components/ui/badge'
import { useHistory } from '@/hooks/use-history'
import { formatMbps } from '@/lib/format'
import { clampPct, signalTone, toneText } from '@/lib/signal'
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

/** A chart with its name in the corner, so it is clear what the line is. The chart already has its own accessible name. */
function Captioned({ caption, value, valueClass, children }: { caption: string; value?: string; valueClass?: string; children: React.ReactNode }) {
  return (
    <div className="relative">
      <span aria-hidden className="pointer-events-none absolute bottom-0.5 left-1 text-[10px] leading-none font-medium uppercase tracking-wider text-muted-foreground">
        {caption}
        {value && <span className={cn('ml-1 font-semibold tabular-nums', valueClass)}>{value}</span>}
      </span>
      {children}
    </div>
  )
}

/** The last couple of minutes of this stream's bitrate. Mounted only while a stream is live, so each new stream starts with a blank line. */
function StreamSparkline({ bps, tick }: { bps?: number; tick: unknown }) {
  const history = useHistory(bps === undefined ? undefined : bps / 1_000_000, tick)
  return (
    <Captioned caption="Bitrate" value={bps ? formatMbps(bps) : undefined}>
      <Sparkline data={history} height={28} label="Bitrate" />
    </Captioned>
  )
}

/** The last couple of minutes of the signal quality the tuner reports for this stream. */
function SignalSparkline({ quality, tick }: { quality: number; tick: unknown }) {
  const history = useHistory(quality, tick)
  return (
    <Captioned caption="Quality" value={`${clampPct(quality)}%`} valueClass={toneText[signalTone(clampPct(quality))]}>
      <Sparkline data={history} height={28} max={100} label="Signal quality" tone="text-chart-2" />
    </Captioned>
  )
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
  /** Where the picture of what is on is served from. */
  posterSrc?: string
  /** The time to measure how far into the program it is: the server's, from the latest state update. */
  now?: Date
}

const clock = (d: Date) => d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

/** How far through a program it is, as a bar with its times and how long is left. Nothing for a time that makes no sense. */
function Progress({ start, stop, now }: { start: string; stop: string; now: Date }) {
  const s = new Date(start)
  const e = new Date(stop)
  const span = e.getTime() - s.getTime()
  if (!(span > 0) || Number.isNaN(now.getTime())) return null
  const done = Math.min(100, Math.max(0, ((now.getTime() - s.getTime()) / span) * 100))
  const left = Math.max(0, Math.ceil((e.getTime() - now.getTime()) / 60_000))
  return (
    <div data-testid="tile-progress" className="space-y-1">
      <div role="progressbar" aria-label="How far into the program" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(done)} className="h-1 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-primary" style={{ width: `${done}%` }} />
      </div>
      <div className="flex justify-between text-[11px] tabular-nums text-muted-foreground">
        <span>
          {clock(s)} – {clock(e)}
        </span>
        <span>{left > 0 ? `${left} min left` : 'ending'}</span>
      </div>
    </div>
  )
}

/** One physical tuner: what's on it and who has it. */
export function TunerTile({ tile, dimmed = false, tick, guide, iconSrc, posterSrc, now }: TileProps) {
  const { slot, owner } = tile
  const live = slot?.inUse === true
  const [failedPoster, setFailedPoster] = useState<string>()
  const program = guide?.now
  const pill = (
    <Badge className={badgeClass[owner.kind]}>
      {owner.kind === 'reserved' && <Crown data-icon="inline-start" />}
      {owner.kind === 'local' && <House data-icon="inline-start" />}
      {(owner.kind === 'viewer' || owner.kind === 'remote') && <Globe data-icon="inline-start" />}
      {ownerLabel(owner)}
    </Badge>
  )

  if (!live) {
    return (
      <div
        data-testid="tuner-tile"
        data-owner={owner.kind}
        className={cn('relative flex h-full min-h-48 flex-col overflow-hidden rounded-2xl border border-dashed border-border bg-muted/30 p-5', dimmed && 'opacity-50')}
      >
        <div className="flex items-center justify-between">
          <span className="font-mono text-xs text-muted-foreground">TUNER {tile.index + 1}</span>
          {pill}
        </div>
        <div className="m-auto text-center text-sm text-muted-foreground">
          {owner.kind === 'reserved' && <Tv className="mx-auto mb-2 size-6" />}
          {owner.kind === 'reserved' ? 'Held for your TVs' : 'Idle'}
        </div>
      </div>
    )
  }

  const showPoster = posterSrc !== undefined && failedPoster !== posterSrc
  const flow = `${slot.channel}|${slot.targetIP}|${owner.kind}`
  const hasSignal = slot.signalStrength !== undefined
  return (
    <div
      data-testid="tuner-tile"
      data-owner={owner.kind}
      className={cn('relative flex h-full flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-sm', dimmed && 'opacity-50')}
    >
      {/* What is on, as a picture when the guide has one: faded into the card, so the text over it is the theme's own. */}
      {showPoster && (
        <img
          src={posterSrc}
          alt=""
          aria-hidden
          onError={() => setFailedPoster(posterSrc)}
          draggable={false}
          className="pointer-events-none absolute inset-x-0 top-0 aspect-video w-full object-cover opacity-40 [mask-image:linear-gradient(to_bottom,black_30%,transparent)] dark:opacity-30"
        />
      )}
      <div className="relative flex aspect-video w-full shrink-0 flex-col justify-between p-4">
        <div className="flex items-center justify-between">
          <span className="font-mono text-xs text-muted-foreground">TUNER {tile.index + 1}</span>
          {pill}
        </div>
        <div className="flex items-end justify-between gap-3">
          <div className="min-w-0">
            <div className="text-4xl font-bold leading-none tracking-tight tabular-nums">{slot.channel ?? '—'}</div>
            <div className="mt-1 truncate text-sm">{slot.channelName ?? 'Tuning…'}</div>
          </div>
          {guide?.icon && iconSrc && (
            <span className="shrink-0 rounded-lg bg-background/60 p-1.5 ring-1 ring-border backdrop-blur">
              <ChannelLogo key={iconSrc} src={iconSrc} name={slot.channelName ?? guide.name ?? 'Channel'} className="h-8 max-w-20" />
            </span>
          )}
        </div>
      </div>

      <div className="relative flex flex-1 flex-col gap-3 px-4 pb-4">
        {program && (
          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between gap-2">
              <div
                data-testid="tile-now"
                className="truncate font-semibold"
                title={program.subTitle ? `${program.title}: ${program.subTitle}` : program.title}
              >
                {program.title}
              </div>
              {program.category && <span className="shrink-0 rounded-full border border-border px-2 py-0.5 text-[10px] text-muted-foreground">{program.category}</span>}
            </div>
            {(program.episode || program.subTitle) && (
              <div className="truncate text-xs text-foreground/80">{[program.episode, program.subTitle].filter(Boolean).join(' · ')}</div>
            )}
            {program.description && <p className="line-clamp-2 text-xs text-muted-foreground">{program.description}</p>}
            <Progress start={program.start} stop={program.stop} now={now ?? new Date(0)} />
          </div>
        )}

        <div className="mt-auto space-y-3">
          <div className={cn('grid gap-3', hasSignal && slot.signalQuality !== undefined ? 'grid-cols-2' : 'grid-cols-1')}>
            <div data-testid="tile-bitrate">
              {/* keyed so a different channel or a different viewer is a new stream with a new line */}
              <StreamSparkline key={flow} bps={slot.bitsPerSecond} tick={tick} />
            </div>
            {hasSignal && slot.signalQuality !== undefined && (
              <div data-testid="tile-signal-trend">
                <SignalSparkline key={flow} quality={slot.signalQuality} tick={tick} />
              </div>
            )}
          </div>
          <div className="flex items-center justify-between gap-3 border-t border-border pt-3 text-xs text-muted-foreground">
            <span className="flex min-w-0 items-center gap-1.5" title="Where it is going">
              {where(tile) && <Network aria-hidden className="size-3.5 shrink-0" />}
              <span className="truncate">{where(tile)}</span>
            </span>
            {hasSignal && (
              <span
                data-testid="tile-signal"
                data-tone={signalTone(clampPct(slot.signalStrength!))}
                title="Signal strength"
                className={cn('flex shrink-0 items-center gap-1.5 tabular-nums', toneText[signalTone(clampPct(slot.signalStrength!))])}
              >
                <Signal aria-hidden className="size-3.5" />
                <span className="sr-only">Signal strength </span>
                {clampPct(slot.signalStrength!)}%
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
