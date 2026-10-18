import { Check, Plus, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useEdits } from '@/hooks/mutations'
import { deviceTitle } from '@/lib/device'
import { parseAddresses } from '@/lib/format'
import type { DeviceState, Viewer, ViewerInput } from '@/lib/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The viewer being edited; undefined to add a new one. */
  viewer?: Viewer
  /** The devices, for the per-device limit fields (shown when there are several). */
  devices: DeviceState[]
}

export function ViewerDialog({ open, onOpenChange, viewer, devices }: Props) {
  // Remount the form whenever the target changes so its fields start fresh.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open && <ViewerForm key={viewer?.name ?? '+'} viewer={viewer} devices={devices} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

const isCount = (s: string) => s.trim() !== '' && Number.isInteger(Number(s)) && Number(s) >= 0

function ViewerForm({ viewer, devices, onDone }: { viewer?: Viewer; devices: DeviceState[]; onDone: () => void }) {
  const several = devices.length > 1
  const mostTuners = Math.max(1, ...devices.map((d) => d.tuner.tunerCount))
  const { add, replace } = useEdits()
  const [name, setName] = useState(viewer?.name ?? '')
  const [addresses, setAddresses] = useState(viewer?.addresses.join('\n') ?? '')
  const [limit, setLimit] = useState(String(viewer?.limit ?? 1))
  // A blank per-device box means "use the default".
  const [overrides, setOverrides] = useState<Record<string, string>>(
    Object.fromEntries(Object.entries(viewer?.limits ?? {}).map(([k, v]) => [k, String(v)])),
  )
  const [error, setError] = useState<string | null>(null)
  const busy = add.isPending || replace.isPending

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: ViewerInput = { name: name.trim(), addresses: parseAddresses(addresses), limit: Number(limit) }
    if (!input.name) return setError('Give the viewer a name.')
    if (input.addresses.length === 0) return setError('Add at least one address.')
    if (!isCount(limit)) return setError('The tuner limit must be 0 or more.')
    const limits: Record<string, number> = {}
    for (const d of devices) {
      const raw = (overrides[d.name] ?? '').trim()
      if (raw === '') continue
      if (!isCount(raw)) return setError(`The limit for ${deviceTitle(d)} must be 0 or more.`)
      limits[d.name] = Number(raw)
    }
    if (several) input.limits = limits // an empty object clears any earlier overrides
    try {
      if (viewer) await replace.mutateAsync({ name: viewer.name, input })
      else await add.mutateAsync(input)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <DialogHeader>
        <DialogTitle>{viewer ? `Edit ${viewer.name}` : 'Add a viewer'}</DialogTitle>
        <DialogDescription>
          A viewer is someone's media server. It is recognised by the address it connects from.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="viewer-name">Name</Label>
        <Input id="viewer-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Mom & Dad" autoFocus />
      </div>
      <div className="space-y-2">
        <Label htmlFor="viewer-addresses">Addresses</Label>
        <Textarea
          id="viewer-addresses"
          value={addresses}
          onChange={(e) => setAddresses(e.target.value)}
          placeholder={'100.67.117.32\n10.8.0.0/24'}
          className="font-mono"
          rows={3}
        />
        <p className="text-xs text-muted-foreground">One per line: a single IP, or a range like 10.8.0.0/24.</p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="viewer-limit">{several ? 'Tuners at once (default)' : 'Tuners at once'}</Label>
        <Input
          id="viewer-limit"
          type="number"
          min={0}
          max={mostTuners}
          value={limit}
          onChange={(e) => setLimit(e.target.value)}
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
                value={overrides[d.name] ?? ''}
                placeholder={limit}
                onChange={(e) => setOverrides({ ...overrides, [d.name]: e.target.value })}
                className="w-24"
              />
            </div>
          ))}
        </fieldset>
      )}

      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          <X data-icon="inline-start" /> Cancel
        </Button>
        <Button type="submit" disabled={busy}>
          {viewer ? <Check data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
          {viewer ? 'Save' : 'Add viewer'}
        </Button>
      </DialogFooter>
    </form>
  )
}
