import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useApp } from '@/hooks/app-context'
import { formatMbps, timeAgo } from '@/lib/format'

export function TunerPage() {
  const { state } = useApp()
  const { tuner } = state
  const facts: [string, string][] = [
    ['Device', tuner.friendlyName ?? 'Unknown'],
    ['Model', tuner.model ?? 'Unknown'],
    ['DeviceID', tuner.id || 'Not yet seen'],
    ['Address', tuner.baseURL.replace(/^https?:\/\//, '')],
    ['Tuners', String(tuner.tunerCount)],
    ['Last seen', timeAgo(tuner.lastSeen)],
  ]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Tuner</h1>
        <p className="mt-1 text-sm text-zinc-400">
          Found by DeviceID, so Antennarr follows it if its address changes.
        </p>
      </div>

      <dl className="grid gap-5 rounded-xl border border-zinc-800 bg-zinc-900/50 p-5 text-sm sm:grid-cols-2 xl:grid-cols-3">
        {facts.map(([k, v]) => (
          <div key={k}>
            <dt className="text-xs text-zinc-500">{k}</dt>
            <dd className="mt-0.5 font-mono text-[13px] font-medium">{v}</dd>
          </div>
        ))}
        <div>
          <dt className="text-xs text-zinc-500">Status</dt>
          <dd className="mt-0.5">
            {tuner.online ? <Badge className="bg-emerald-500/15 text-emerald-300">Online</Badge> : <Badge className="bg-red-500/15 text-red-300">Offline</Badge>}
          </dd>
        </div>
      </dl>

      <div className="rounded-xl border border-zinc-800 bg-zinc-900/50">
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
                <TableCell colSpan={5} className="text-center text-zinc-500">
                  No tuner data yet.
                </TableCell>
              </TableRow>
            )}
            {tuner.slots.map((s) => (
              <TableRow key={s.resource}>
                <TableCell className="font-mono text-xs">{s.resource}</TableCell>
                <TableCell>{s.inUse ? `${s.channel ?? '—'} ${s.channelName ?? ''}` : <span className="text-zinc-500">idle</span>}</TableCell>
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
    </div>
  )
}
