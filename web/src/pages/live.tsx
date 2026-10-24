import { CircleCheck, Clock, RadioReceiver, Users } from 'lucide-react'
import { Link } from 'react-router'
import { FirmwareBadge } from '@/components/firmware'
import { tunerPath } from '@/lib/tuner-tabs'
import { NoTuners } from '@/components/no-tuners'
import { TunerTile } from '@/components/tuner-tile'
import { Badge } from '@/components/ui/badge'
import { useApp } from '@/hooks/app-context'
import { channelIconUrl, programPosterUrl } from '@/lib/api'
import { deviceModel, deviceTitle } from '@/lib/device'
import { formatMbps, timeAgo } from '@/lib/format'
import { recentProblems } from '@/lib/problems'
import { buildTiles, limitFor } from '@/lib/slots'
import type { DeviceState, EventKind, TunerEvent, Viewer } from '@/lib/types'
import { cn } from '@/lib/utils'

const dot: Record<EventKind, string> = {
  ok: 'bg-emerald-400',
  info: 'bg-cyan-400',
  warn: 'bg-amber-400',
  err: 'bg-red-400',
}

const sum = <T,>(xs: T[], f: (x: T) => number) => xs.reduce((a, x) => a + f(x), 0)

export function LivePage() {
  const { state } = useApp()
  const { devices, viewers } = state
  const several = devices.length > 1

  const slots = devices.flatMap((d) => d.tuner.slots)
  const totalBps = sum(slots, (s) => s.bitsPerSecond ?? 0)

  const parts = [
    `${sum(devices, (d) => d.summary.remote)} remote`,
    `${sum(devices, (d) => d.summary.local)} local`,
    `${sum(devices, (d) => d.summary.reserved)} held for you`,
    formatMbps(totalBps),
  ]

  // Nothing wrong anywhere: the tuners answer and have their channels. Worth saying, so that someone who only looks
  // when something seems broken can see that it isn't Yagi. A viewer turned away at their limit, or every tuner busy,
  // is Yagi doing its job and is told separately at the top of the page; it is not a fault.
  const working = devices.length > 0 && devices.every((d) => d.tuner.online && !d.tuner.updating && !d.tuner.restarting) && recentProblems(state).every(({ p }) => p.kind !== 'no-channels')
  const where = devices.length === 1 ? ` The tuner is at ${devices[0].tuner.baseURL.replace(/^https?:\/\//, '').replace(/\/$/, '')}.` : ''

  // Events from every device, newest first, tagged with where they happened.
  const events: (TunerEvent & { device: string })[] = devices
    .flatMap((d) => d.tuner.events.map((e) => ({ ...e, device: deviceTitle(d) })))
    .sort((a, b) => new Date(b.time).getTime() - new Date(a.time).getTime())

  return (
    <div className="space-y-8">
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div>
          <div className="text-xs tracking-[0.2em] text-cyan-700 dark:text-cyan-300/80 uppercase">On the air now</div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">
            {devices.length === 0 ? 'No tuners yet' : `${sum(devices, (d) => d.summary.inUse)} of ${sum(devices, (d) => d.summary.total)} tuners live`}
          </h1>
          {devices.length > 0 && <p className="mt-1 text-muted-foreground">{parts.join(' · ')}</p>}
          {working && (
            <p className="mt-2 flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-300">
              <CircleCheck aria-hidden className="size-4 shrink-0" />
              <span>
                <strong className="font-medium">{several ? `All ${devices.length} tuners are working.` : 'Everything is working.'}</strong>
                {where}
              </span>
            </p>
          )}
        </div>
      </div>

      {devices.length === 0 && <NoTuners />}
      {devices.map((d) => (
        <DeviceSection key={d.name} device={d} tick={state.time} />
      ))}

      <div className="grid gap-6 lg:grid-cols-2">
        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <Users aria-hidden className="size-4 shrink-0 text-muted-foreground" />
            Viewers
          </h2>
          {viewers.length === 0 ? (
            <p className="rounded-xl border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
              No viewers yet. Anyone who can reach {several ? 'these ports' : 'this port'} may use {several ? 'the tuners' : 'the tuner'}.
            </p>
          ) : (
            <div className="space-y-2">
              {viewers.map((v) => (
                <ViewerRow key={v.name} v={v} devices={devices} />
              ))}
            </div>
          )}
        </section>

        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <Clock aria-hidden className="size-4 shrink-0 text-muted-foreground" />
            Timeline
          </h2>
          {events.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nothing has happened yet.</p>
          ) : (
            <ol className="relative ml-2 space-y-5 border-l border-border">
              {events.map((e, i) => (
                <li key={`${e.device}-${e.time}-${i}`} className="relative pl-5">
                  <span className={cn('absolute top-1.5 -left-[5px] size-2.5 rounded-full ring-4 ring-background', dot[e.kind])} />
                  <div className="text-sm">{e.text}</div>
                  <div className="text-xs text-muted-foreground">
                    {timeAgo(e.time)}
                    {several && ` · ${e.device}`}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </div>
  )
}

function DeviceSection({ device, tick }: { device: DeviceState; tick: unknown }) {
  const { tuner, summary } = device
  const model = deviceModel(device)
  return (
    <section aria-label={`Tuners on ${deviceTitle(device)}`} className="space-y-3">
      {/* The top bar doesn't say anything about the tuners, so each one has its header here: its name, the model beside a name you chose, how many are live, and where it is. */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold tracking-tight">
          <span
            title={tuner.updating ? 'Updating firmware' : tuner.restarting ? 'Restarting' : tuner.online ? 'Online' : 'Offline'}
            className={cn('size-2 rounded-full', tuner.updating || tuner.restarting ? 'animate-pulse bg-amber-400' : tuner.online ? 'bg-emerald-400' : 'bg-red-400')}
          />
          <Link to={tunerPath(device.name)} title="This tuner's page" className="underline-offset-4 hover:underline">
            {deviceTitle(device)}
          </Link>
        </h2>
        {tuner.updating && <Badge className="bg-amber-500/15 text-amber-800 dark:text-amber-300">Updating firmware…</Badge>}
        {tuner.restarting && <Badge className="bg-amber-500/15 text-amber-800 dark:text-amber-300">Restarting…</Badge>}
        <span className="text-sm text-muted-foreground">
          {model && `${model} · `}
          {summary.inUse} of {summary.total} live
        </span>
        <span className="font-mono text-xs text-muted-foreground">{tuner.baseURL.replace(/^https?:\/\//, '')}</span>
        <FirmwareBadge status={device.firmware} to={tunerPath(device.name, 'firmware')} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {buildTiles(device).map((t) => (
          <TunerTile
            key={t.index}
            tile={t}
            dimmed={!tuner.online}
            tick={tick}
            now={new Date(String(tick))}
            guide={t.slot?.channel ? device.guide[t.slot.channel] : undefined}
            iconSrc={t.slot?.channel ? channelIconUrl(device.name, t.slot.channel) : undefined}
            posterSrc={
              t.slot?.channel && device.guide[t.slot.channel]?.now?.poster
                ? programPosterUrl(device.name, t.slot.channel, device.guide[t.slot.channel].now!.poster!)
                : undefined
            }
          />
        ))}
      </div>
    </section>
  )
}

function ViewerRow({ v, devices }: { v: Viewer; devices: DeviceState[] }) {
  const several = devices.length > 1
  return (
    <div className="flex items-center gap-4 rounded-xl border border-border bg-card/70 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 font-medium">
          {v.name}
          {v.inUse > 0 && <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">Watching</Badge>}
        </div>
        <div className="truncate text-xs text-muted-foreground">
          {v.addresses.join(', ')} · {v.lastSeen ? `seen ${timeAgo(v.lastSeen)}` : 'waiting for first contact'}
        </div>
      </div>
      <div className="flex flex-col items-end gap-1.5">
        {devices.map((d) => {
          const limit = limitFor(v, d.name)
          const inUse = v.inUseByDevice?.[d.name] ?? 0
          return (
            <div key={d.name} className="flex items-center gap-2">
              {several && <span className="text-xs text-muted-foreground">{deviceTitle(d)}</span>}
              <div className="flex gap-1" role="img" aria-label={`${several ? `${deviceTitle(d)}: ` : ''}${inUse} of ${limit} tuners in use`}>
                {Array.from({ length: limit }).map((_, i) => (
                  <RadioReceiver key={i} className={cn('size-6', i < inUse ? 'text-cyan-400' : 'text-muted-foreground/40')} />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
