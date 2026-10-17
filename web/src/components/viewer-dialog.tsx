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
import { parseAddresses } from '@/lib/format'
import type { Viewer } from '@/lib/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The viewer being edited; undefined to add a new one. */
  viewer?: Viewer
  /** How many tuners the device has, to bound the limit field. */
  tunerCount: number
}

export function ViewerDialog({ open, onOpenChange, viewer, tunerCount }: Props) {
  // Remount the form whenever the target changes so its fields start fresh.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open && <ViewerForm key={viewer?.name ?? '+'} viewer={viewer} tunerCount={tunerCount} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function ViewerForm({ viewer, tunerCount, onDone }: { viewer?: Viewer; tunerCount: number; onDone: () => void }) {
  const { add, replace } = useEdits()
  const [name, setName] = useState(viewer?.name ?? '')
  const [addresses, setAddresses] = useState(viewer?.addresses.join('\n') ?? '')
  const [limit, setLimit] = useState(String(viewer?.limit ?? 1))
  const [error, setError] = useState<string | null>(null)
  const busy = add.isPending || replace.isPending

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input = { name: name.trim(), addresses: parseAddresses(addresses), limit: Number(limit) }
    if (!input.name) return setError('Give the viewer a name.')
    if (input.addresses.length === 0) return setError('Add at least one address.')
    if (!Number.isInteger(input.limit) || input.limit < 0) return setError('The tuner limit must be 0 or more.')
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
        <Label htmlFor="viewer-limit">Tuners at once</Label>
        <Input
          id="viewer-limit"
          type="number"
          min={0}
          max={Math.max(tunerCount, 1)}
          value={limit}
          onChange={(e) => setLimit(e.target.value)}
          className="w-24"
        />
        <p className="text-xs text-muted-foreground">0 pauses this viewer without removing them.</p>
      </div>

      {error && (
        <p role="alert" className="text-sm text-red-400">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          Cancel
        </Button>
        <Button type="submit" disabled={busy}>
          {viewer ? 'Save' : 'Add viewer'}
        </Button>
      </DialogFooter>
    </form>
  )
}
