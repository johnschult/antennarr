import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useApp } from '@/hooks/app-context'
import { ChannelLogo } from '@/components/channel-logo'
import { FirmwareSection } from '@/components/firmware'
import { channelIconUrl } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import { formatMbps, timeAgo } from '@/lib/format'
import type { DeviceState } from '@/lib/types'

export function TunerPage() {
  const { state } = useApp()
  const several = state.devices.length > 1
  return (
    <div className="space-y-10">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{several ? 'Tuners' : (state.devices[0].label ?? 'Tuner')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">Found by DeviceID, so Antennarr follows a tuner if its address changes.</p>
      </div>
      {state.devices.map((d) => (
        <DeviceDetails key={d.name} device={d} showName={several} />
      ))}
    </div>
  )
}

function DeviceDetails({ device, showName }: { device: DeviceState; showName: boolean }) {
  const { tuner } = device
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
                  {s.inUse ? (
                    <span className="flex items-center gap-2">
                      {s.channel && device.guide[s.channel]?.icon && (
                        <ChannelLogo key={s.channel} src={channelIconUrl(device.name, s.channel)} name={s.channelName ?? device.guide[s.channel].name ?? 'Channel'} className="h-6 max-w-12" />
                      )}
                      {`${s.channel ?? '—'} ${s.channelName ?? ''}`}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">idle</span>
                  )}
                </TableCell>
                <TableCell>{s.inUse ? (s.viaAntennarr ? 'Antennarr' : (s.targetIP ?? 'Local device')) : '—'}</TableCell>
                <TableCell className="tabular-nums">
                  {s.inUse && s.signalStrength !== undefined ? `${s.signalStrength}% / ${s.signalQuality ?? 0}%` : '—'}
                </TableCell>
                <TableCell className="tabular-nums">{s.inUse ? formatMbps(s.bitsPerSecond) : '—'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}
