import { NoTuners } from '@/components/no-tuners'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useApp } from '@/hooks/app-context'
import { useIsPhone } from '@/hooks/use-media-query'
import { ChannelLogo } from '@/components/channel-logo'
import { FirmwareSection } from '@/components/firmware'
import { channelIconUrl } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import { formatMbps, timeAgo } from '@/lib/format'
import type { DeviceState, Slot } from '@/lib/types'

export function TunerPage() {
  const { state } = useApp()
  const several = state.devices.length > 1
  return (
    <div className="space-y-10">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{several || state.devices.length === 0 ? 'Tuners' : (state.devices[0].label ?? 'Tuner')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">Found by DeviceID, so Antennarr follows a tuner if its address changes.</p>
      </div>
      {state.devices.length === 0 && <NoTuners />}
      {state.devices.map((d) => (
        <DeviceDetails key={d.name} device={d} showName={several} />
      ))}
    </div>
  )
}

/** The channel on a tuner, with its logo when the guide has one, or "idle". */
function ChannelCell({ s, device }: { s: Slot; device: DeviceState }) {
  if (!s.inUse) return <span className="text-muted-foreground">idle</span>
  return (
    <span className="flex items-center gap-2">
      {s.channel && device.guide[s.channel]?.icon && (
        <ChannelLogo key={s.channel} src={channelIconUrl(device.name, s.channel)} name={s.channelName ?? device.guide[s.channel].name ?? 'Channel'} className="h-6 max-w-12" />
      )}
      {`${s.channel ?? '—'} ${s.channelName ?? ''}`}
    </span>
  )
}

const usedBy = (s: Slot) => (s.inUse ? (s.viaAntennarr ? 'Antennarr' : (s.targetIP ?? 'Local device')) : '—')
const signalOf = (s: Slot) => (s.inUse && s.signalStrength !== undefined ? `${s.signalStrength}% / ${s.signalQuality ?? 0}%` : '—')
const rateOf = (s: Slot) => (s.inUse ? formatMbps(s.bitsPerSecond) : '—')

function DeviceDetails({ device, showName }: { device: DeviceState; showName: boolean }) {
  const { tuner } = device
  const phone = useIsPhone()
  const facts: [string, string][] = [
    ['Device', tuner.friendlyName ?? 'Unknown'],
    ['Model', tuner.model ?? 'Unknown'],
    ['DeviceID', tuner.id || 'Not yet seen'],
    ['Address', tuner.baseURL.replace(/^https?:\/\//, '')],
    ['Tuners', String(tuner.tunerCount)],
    ['Last seen', timeAgo(tuner.lastSeen)],
  ]

  return (
    <section aria-label={showName ? deviceTitle(device) : undefined} className="space-y-4">
      {showName && <h2 className="text-lg font-semibold tracking-tight">{deviceTitle(device)}</h2>}

      <dl className="grid gap-5 rounded-xl border border-border bg-card/70 p-5 text-sm sm:grid-cols-2 xl:grid-cols-3">
        {facts.map(([k, v]) => (
          <div key={k}>
            <dt className="text-xs text-muted-foreground">{k}</dt>
            <dd className="mt-0.5 font-mono text-[13px] font-medium">{v}</dd>
          </div>
        ))}
        <div>
          <dt className="text-xs text-muted-foreground">Status</dt>
          <dd className="mt-0.5">
            {tuner.online ? <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">Online</Badge> : <Badge className="bg-red-500/15 text-red-700 dark:text-red-300">Offline</Badge>}
          </dd>
        </div>
      </dl>

      <FirmwareSection device={device} />

      {phone ? (
        <ul aria-label="Tuners" className="space-y-3">
          {tuner.slots.length === 0 && <li className="rounded-xl border border-border bg-card/70 p-4 text-center text-sm text-muted-foreground">No tuner data yet.</li>}
          {tuner.slots.map((s) => (
            <li key={s.resource} className="rounded-xl border border-border bg-card/70 p-4">
              <div className="flex items-center justify-between gap-3">
                <span className="font-mono text-xs text-muted-foreground">{s.resource}</span>
                <ChannelCell s={s} device={device} />
              </div>
              {s.inUse && (
                <dl className="mt-3 grid grid-cols-3 gap-3 text-sm">
                  <div>
                    <dt className="text-xs text-muted-foreground">Used by</dt>
                    <dd className="mt-0.5 break-words">{usedBy(s)}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">Signal</dt>
                    <dd className="mt-0.5 tabular-nums">{signalOf(s)}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">Rate</dt>
                    <dd className="mt-0.5 tabular-nums">{rateOf(s)}</dd>
                  </div>
                </dl>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <div className="rounded-xl border border-border bg-card/70">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Tuner</TableHead>
                <TableHead>Channel</TableHead>
                <TableHead>Used by</TableHead>
                <TableHead>Signal</TableHead>
                <TableHead>Rate</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tuner.slots.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    No tuner data yet.
                  </TableCell>
                </TableRow>
              )}
              {tuner.slots.map((s) => (
                <TableRow key={s.resource}>
                  <TableCell className="font-mono text-xs">{s.resource}</TableCell>
                  <TableCell>
                    <ChannelCell s={s} device={device} />
                  </TableCell>
                  <TableCell>{usedBy(s)}</TableCell>
                  <TableCell className="tabular-nums">{signalOf(s)}</TableCell>
                  <TableCell className="tabular-nums">{rateOf(s)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}
