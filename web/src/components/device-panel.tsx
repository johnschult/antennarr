import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, Server } from 'lucide-react'
import { useState } from 'react'
import { Section } from '@/components/settings/section'
import { Button } from '@/components/ui/button'
import { getDeviceInfo } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import { firmwareDate, timeAgo } from '@/lib/format'
import type { DeviceState } from '@/lib/types'

/** What the tuner says about itself. Read when the panel opens and when asked again, not on a timer. */
export function DevicePanel({ device, showName }: { device: DeviceState; showName: boolean }) {
  const qc = useQueryClient()
  const key = ['device-info', device.name] as const
  const online = device.tuner.online
  const query = useQuery({
    queryKey: key,
    queryFn: () => getDeviceInfo(device.name),
    enabled: online,
    staleTime: 30_000,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const [refreshing, setRefreshing] = useState(false)
  const refresh = async () => {
    setRefreshing(true)
    try {
      qc.setQueryData(key, await getDeviceInfo(device.name, true))
    } catch {
      await qc.invalidateQueries({ queryKey: key })
    } finally {
      setRefreshing(false)
    }
  }

  const info = query.data
  const fields: [string, string | undefined][] = info
    ? [
        ['Hardware model', info.hardwareModel],
        ['Firmware', info.version ? `${firmwareDate(info.version)} · ${info.version}` : undefined],
        ['Firmware family', info.model],
        ['ATSC 3.0 DRM', info.drm],
        ['Device ID', info.deviceId ?? device.tuner.id],
        ['MAC address', info.mac],
        ['IP address', info.ip],
        ['Subnet mask', info.subnet],
        ['Channel maps', info.channelMaps?.join(', ')],
        ['Modulation', info.modulations?.join(', ')],
      ]
    : []

  return (
    <Section icon={Server} title={showName ? `Device — ${deviceTitle(device)}` : 'Device'}>
      <div className="px-4 py-3.5">
        {!online ? (
          <p className="text-sm text-muted-foreground">The tuner is offline, so there is nothing to read.</p>
        ) : query.isPending ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 aria-hidden className="size-4 animate-spin" /> Asking the tuner…
          </p>
        ) : query.isError ? (
          <p role="alert" className="text-sm text-red-700 dark:text-red-300">
            {query.error instanceof Error ? query.error.message : "Couldn't read the tuner."}
          </p>
        ) : (
          <dl className="grid gap-x-8 gap-y-2.5 text-sm sm:grid-cols-2">
            {fields
              .filter(([, v]) => v)
              .map(([label, value]) => (
                <div key={label} className="min-w-0">
                  <dt className="text-xs text-muted-foreground">{label}</dt>
                  <dd className="break-words font-mono text-[13px]">{value}</dd>
                </div>
              ))}
          </dl>
        )}
        {info?.problems?.map((p) => (
          <p key={p} className="mt-3 text-xs text-muted-foreground">
            Not shown: {p}.
          </p>
        ))}
      </div>
      {online && (
        <div className="flex items-center justify-between gap-3 px-4 py-2.5 text-xs text-muted-foreground">
          <span>{info ? `Updated ${timeAgo(info.readAt)}. Read-only: nothing here changes the tuner.` : 'Read-only: nothing here changes the tuner.'}</span>
          <Button size="sm" variant="outline" disabled={refreshing || query.isPending} onClick={refresh}>
            {refreshing ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <RefreshCw data-icon="inline-start" />}
            {refreshing ? 'Refreshing…' : 'Refresh'}
          </Button>
        </div>
      )}
    </Section>
  )
}
