import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { ViewerFieldsState } from '@/hooks/use-viewer-fields'
import { deviceTitle } from '@/lib/device'
import type { DeviceState } from '@/lib/types'

export function ViewerFields({ f, devices }: { f: ViewerFieldsState; devices: DeviceState[] }) {
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="viewer-name">Name</Label>
        <Input id="viewer-name" value={f.name} onChange={(e) => f.setName(e.target.value)} placeholder="Mom & Dad" autoFocus />
      </div>
      <div className="space-y-2">
        <Label htmlFor="viewer-addresses">Addresses</Label>
        <Textarea
          id="viewer-addresses"
          value={f.addresses}
          onChange={(e) => f.setAddresses(e.target.value)}
          placeholder={'100.67.117.32\n10.8.0.0/24'}
          className="font-mono"
          rows={3}
        />
        <p className="text-xs text-muted-foreground">
          One per line: a single IP, or a range like 10.8.0.0/24. Yagi serves private networks only (your network, a VPN, Tailscale), so these are the
          addresses it sees from there, never public internet ones.
        </p>
      </div>
      <LimitFields f={f} devices={devices} />
    </>
  )
}

/** How many tuners at once, and the same per device when there are several. Shared by a viewer and an invite. */
export function LimitFields({ f, devices }: { f: Pick<ViewerFieldsState, 'limit' | 'setLimit' | 'overrides' | 'setOverrides'>; devices: DeviceState[] }) {
  const several = devices.length > 1
  const mostTuners = Math.max(1, ...devices.map((d) => d.tuner.tunerCount))
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="viewer-limit">{several ? 'Tuners at once (default)' : 'Tuners at once'}</Label>
        <Input
          id="viewer-limit"
          type="number"
          min={0}
          max={mostTuners}
          value={f.limit}
          onChange={(e) => f.setLimit(e.target.value)}
          className="w-24"
        />
        <p className="text-xs text-muted-foreground">
          {several ? 'On each device, unless you set one below. ' : ''}0 pauses this viewer without removing them.
        </p>
      </div>
      {several && (
        <fieldset className="space-y-2 rounded-lg border border-border p-3">
          <legend className="px-1 text-xs text-muted-foreground">Per device (leave blank to use the default)</legend>
          {devices.map((d) => (
            <div key={d.name} className="flex items-center gap-3">
              <Label htmlFor={`viewer-limit-${d.name}`} className="w-28 shrink-0">
                Tuners on {deviceTitle(d)}
              </Label>
              <Input
                id={`viewer-limit-${d.name}`}
                type="number"
                min={0}
                max={Math.max(d.tuner.tunerCount, 1)}
                value={f.overrides[d.name] ?? ''}
                placeholder={f.limit}
                onChange={(e) => f.setOverrides({ ...f.overrides, [d.name]: e.target.value })}
                className="w-24"
              />
            </div>
          ))}
        </fieldset>
      )}
    </>
  )
}
