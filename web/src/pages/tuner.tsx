import { ChevronDown, ChevronRight, Plus, SlidersHorizontal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { AddTunerDialog } from '@/components/add-tuner-dialog'
import { NoTuners } from '@/components/no-tuners'
import { RemoveTunerDialog } from '@/components/remove-tuner-dialog'
import { EditRow } from '@/components/settings/edit-row'
import { Row, Section } from '@/components/settings/section'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { moveToPort } from '@/lib/navigate'
import { ChannelLogo } from '@/components/channel-logo'
import { DevicePanel } from '@/components/device-panel'
import { FirmwareBadge, FirmwareSection } from '@/components/firmware'
import { channelIconUrl } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import { formatMbps, timeAgo } from '@/lib/format'
import type { DeviceState, Slot } from '@/lib/types'

const asset = (name: string) => `${import.meta.env.BASE_URL}${name}`

export function TunerPage() {
  const { state } = useApp()
  const several = state.devices.length > 1
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<DeviceState | null>(null)
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Tuners</h1>
          <p className="mt-1 text-sm text-muted-foreground">Found by DeviceID, so Antennarr follows a tuner if its address changes.</p>
        </div>
        {state.devices.length > 0 && (
          <div className="flex flex-col items-end gap-1">
            <Button size="sm" disabled={!state.editable} onClick={() => setAdding(true)}>
              <Plus data-icon="inline-start" /> Add tuner
            </Button>
            {!state.editable && <p className="max-w-xs text-right text-xs text-muted-foreground">Antennarr has no settings file to save tuners in.</p>}
          </div>
        )}
      </div>
      {state.devices.length === 0 && <NoTuners />}
      {state.devices.map((d) => (
        <DeviceDetails key={d.name} device={d} showName={several} onRemove={() => setRemoving(d)} />
      ))}
      <AddTunerDialog open={adding} onOpenChange={setAdding} />
      <RemoveTunerDialog device={removing} onClose={() => setRemoving(null)} />
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

const validPort = (text: string) => {
  const n = Number(text)
  return text.trim() === '' || !Number.isInteger(n) || n < 1 || n > 65535 ? 'Enter a port from 1 to 65535.' : null
}

const usedBy = (s: Slot) => (s.viaAntennarr ? 'Antennarr' : (s.targetIP ?? 'Local device'))
const signalOf = (s: Slot) => (s.signalStrength !== undefined ? `${s.signalStrength}% / ${s.signalQuality ?? 0}%` : null)

/** One tuner's slot: idle, or the channel on it, who has it, its signal and its rate. */
function SlotCell({ s, device }: { s: Slot; device: DeviceState }) {
  const signal = signalOf(s)
  return (
    <li className="rounded-lg border border-border bg-background/40 px-3 py-2 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-xs text-muted-foreground">{s.resource}</span>
        <ChannelCell s={s} device={device} />
      </div>
      {s.inUse && (
        <div className="mt-1.5 flex flex-wrap items-center justify-between gap-x-3 text-xs text-muted-foreground">
          <span className="break-words">{usedBy(s)}</span>
          <span className="tabular-nums">{[signal, formatMbps(s.bitsPerSecond)].filter(Boolean).join(' · ')}</span>
        </div>
      )}
    </li>
  )
}

function DeviceDetails({ device, showName, onRemove }: { device: DeviceState; showName: boolean; onRemove: () => void }) {
  const { tuner } = device
  const { state } = useApp()
  const { device: edit } = useEdits()
  // One tuner has the page to itself; several start folded so they can be scanned.
  const [open, setOpen] = useState(!showName)
  const port = device.listen.slice(device.listen.lastIndexOf(':') + 1)
  const busy = tuner.slots.filter((s) => s.inUse).length
  const bodyId = `tuner-${device.name}`
  const facts: [string, string][] = [
    ['Device', tuner.friendlyName ?? 'Unknown'],
    ['Model', tuner.model ?? 'Unknown'],
    ['DeviceID', tuner.id || 'Not yet seen'],
    ['Address', tuner.baseURL.replace(/^https?:\/\//, '')],
    ['Tuners', String(tuner.tunerCount)],
    ['Last seen', timeAgo(tuner.lastSeen)],
  ]

  return (
    <section aria-label={showName ? deviceTitle(device) : undefined} className="rounded-xl border border-border bg-card/70">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen(!open)}
          className="flex min-w-0 flex-1 items-center gap-3 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {open ? <ChevronDown className="size-4 shrink-0 text-muted-foreground" aria-hidden /> : <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
          <span
            aria-hidden
            title={tuner.updating ? 'Updating firmware' : tuner.online ? 'Online' : 'Offline'}
            className={`size-2.5 shrink-0 rounded-full ${tuner.updating ? 'animate-pulse bg-amber-500' : tuner.online ? 'bg-emerald-500' : 'bg-red-500'}`}
          />
          <img src={asset('hdhomerun-icon.svg')} alt="" aria-hidden="true" className="size-8 shrink-0 rounded-md" draggable={false} />
          <span className="min-w-0">
            <span className="block truncate font-semibold tracking-tight">{deviceTitle(device)}</span>
            <span className="block truncate font-mono text-xs text-muted-foreground">
              {[tuner.friendlyName !== deviceTitle(device) ? tuner.friendlyName : undefined, tuner.baseURL.replace(/^https?:\/\//, ''), `port ${port}`].filter(Boolean).join(' · ')}
            </span>
          </span>
        </button>
        <span className="text-sm tabular-nums text-muted-foreground">
          {busy} of {tuner.tunerCount} in use
        </span>
        <FirmwareBadge status={device.firmware} />
      </div>

      {open && (
        <div id={bodyId} className="space-y-5 border-t border-border p-4">
          <dl className="flex flex-wrap gap-x-8 gap-y-3 text-sm">
            {facts.map(([k, v]) => (
              <div key={k}>
                <dt className="text-xs text-muted-foreground">{k}</dt>
                <dd className="mt-0.5 font-mono text-[13px] font-medium">{v}</dd>
              </div>
            ))}
            <div>
              <dt className="text-xs text-muted-foreground">Status</dt>
              <dd className="mt-0.5">
                {tuner.updating ? (
                  <Badge className="bg-amber-500/15 text-amber-800 dark:text-amber-300">Updating firmware…</Badge>
                ) : tuner.online ? (
                  <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">Online</Badge>
                ) : (
                  <Badge className="bg-red-500/15 text-red-700 dark:text-red-300">Offline</Badge>
                )}
              </dd>
            </div>
          </dl>

          {tuner.slots.length === 0 ? (
            <p className="text-sm text-muted-foreground">No tuner data yet.</p>
          ) : (
            <ul aria-label={showName ? `Tuners on ${deviceTitle(device)}` : 'Tuners'} className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
              {tuner.slots.map((s) => (
                <SlotCell key={s.resource} s={s} device={device} />
              ))}
            </ul>
          )}

          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Section icon={SlidersHorizontal} title={showName ? `Manage ${deviceTitle(device)}` : 'Manage'}>
              <EditRow
                key={`label-${device.name}-${device.label ?? ''}`}
                kind="text"
                maxLength={60}
                id={`setting-${device.name}-label`}
                title="Name"
                description="What it is called on this page. Plex and other media servers always see the device, e.g. HDHomeRun FLEX 4K."
                initial={device.label ?? ''}
                placeholder={tuner.friendlyName ?? device.name}
                disabled={!state.editable}
                onSave={(text) => edit.mutateAsync({ name: device.name, patch: { label: text.trim() } })}
              />
              <EditRow
                key={`port-${device.name}-${device.listen}`}
                kind="number"
                min={1}
                max={65535}
                id={`setting-${device.name}-port`}
                title="Port"
                description="Where media servers reach it. Changing it cuts running streams, and this page moves there."
                initial={port}
                validate={validPort}
                disabled={!state.editable}
                onSave={async (text) => {
                  const host = device.listen.slice(0, device.listen.lastIndexOf(':'))
                  await edit.mutateAsync({ name: device.name, patch: { listen: `${host}:${Number(text)}` } })
                  moveToPort(Number(text))
                }}
              />
              <Row title="Remove this tuner" description="Stops sharing it and cuts its streams. The tuner itself is untouched.">
                <Button variant="destructive" size="sm" disabled={!state.editable} aria-label={`Remove ${deviceTitle(device)}`} onClick={onRemove}>
                  <Trash2 data-icon="inline-start" /> Remove
                </Button>
              </Row>
            </Section>

            <DevicePanel device={device} showName={showName} />

            <FirmwareSection device={device} />
          </div>
        </div>
      )}
    </section>
  )
}
