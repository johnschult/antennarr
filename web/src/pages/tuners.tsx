import { ChevronRight, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { AddTunerDialog } from '@/components/add-tuner-dialog'
import { FirmwareBadge } from '@/components/firmware'
import { NoTuners } from '@/components/no-tuners'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'
import { tunerPath } from '@/lib/tuner-tabs'
import type { DeviceState } from '@/lib/types'

const asset = (name: string) => `${import.meta.env.BASE_URL}${name}`

/** A tuner's state as a dot: pulsing amber while it restarts or updates, green online, red offline. */
export function StatusDot({ device, className = 'size-2.5' }: { device: DeviceState; className?: string }) {
  const { tuner } = device
  return (
    <span
      aria-hidden
      title={tuner.updating ? 'Updating firmware' : tuner.restarting ? 'Restarting' : tuner.online ? 'Online' : 'Offline'}
      className={`${className} shrink-0 rounded-full ${tuner.updating || tuner.restarting ? 'animate-pulse bg-amber-500' : tuner.online ? 'bg-emerald-500' : 'bg-red-500'}`}
    />
  )
}

/** What a tuner is, under its name: the device if the name is your own, where it is and the port. */
export function tunerSubtitle(device: DeviceState): string {
  const port = device.listen.slice(device.listen.lastIndexOf(':') + 1)
  return [device.tuner.friendlyName !== deviceTitle(device) ? device.tuner.friendlyName : undefined, device.tuner.baseURL.replace(/^https?:\/\//, ''), `port ${port}`].filter(Boolean).join(' · ')
}

function TunerRow({ device }: { device: DeviceState }) {
  const busy = device.tuner.slots.filter((s) => s.inUse).length
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border border-border bg-card/70 px-4 py-3">
      <Link to={tunerPath(device.name)} className="group flex min-w-0 flex-1 items-center gap-3 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <StatusDot device={device} />
        <img src={asset('hdhomerun-icon.svg')} alt="" aria-hidden="true" className="size-8 shrink-0 rounded-md" draggable={false} />
        <span className="min-w-0">
          <span className="block truncate font-semibold tracking-tight group-hover:underline">{deviceTitle(device)}</span>
          <span className="block truncate font-mono text-xs text-muted-foreground">{tunerSubtitle(device)}</span>
        </span>
      </Link>
      <span className="text-sm tabular-nums text-muted-foreground">
        {busy} of {device.tuner.tunerCount} in use
      </span>
      <FirmwareBadge status={device.firmware} to={tunerPath(device.name, 'firmware')} />
      <ChevronRight aria-hidden className="size-4 text-muted-foreground" />
    </li>
  )
}

/** The tuners being shared, one row each; a row opens that tuner's page. Even with only one, so there is always a way to add another. */
export function TunersPage() {
  const { state } = useApp()
  const [adding, setAdding] = useState(false)
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Tuners</h1>
          <p className="mt-1 text-sm text-muted-foreground">Found by DeviceID, so Yagi follows a tuner if its address changes.</p>
        </div>
        {state.devices.length > 0 && (
          <div className="flex flex-col items-end gap-1">
            <Button size="sm" disabled={!state.editable} onClick={() => setAdding(true)}>
              <Plus data-icon="inline-start" /> Add tuner
            </Button>
            {!state.editable && <p className="max-w-xs text-right text-xs text-muted-foreground">Yagi has no settings file to save tuners in.</p>}
          </div>
        )}
      </div>
      {state.devices.length === 0 && <NoTuners />}
      <ul aria-label="Tuners" className="space-y-2">
        {state.devices.map((d) => (
          <TunerRow key={d.name} device={d} />
        ))}
      </ul>
      <AddTunerDialog open={adding} onOpenChange={setAdding} />
    </div>
  )
}
