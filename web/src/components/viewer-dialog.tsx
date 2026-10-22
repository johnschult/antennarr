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
import { ViewerFields } from '@/components/viewer-fields'
import { useEdits } from '@/hooks/mutations'
import { useViewerFields } from '@/hooks/use-viewer-fields'
import type { DeviceState, Viewer } from '@/lib/types'

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

function ViewerForm({ viewer, devices, onDone }: { viewer?: Viewer; devices: DeviceState[]; onDone: () => void }) {
  const { add, replace } = useEdits()
  const fields = useViewerFields(viewer, devices)
  const [error, setError] = useState<string | null>(null)
  const busy = add.isPending || replace.isPending

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const built = fields.build()
    if ('error' in built) return setError(built.error)
    try {
      if (viewer) await replace.mutateAsync({ name: viewer.name, input: built.input })
      else await add.mutateAsync(built.input)
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
          A viewer is someone's media server. It is recognized by the address it connects from.
        </DialogDescription>
      </DialogHeader>

      {viewer?.plex && (
        <p className="rounded-lg border border-border bg-muted/50 p-3 text-xs text-muted-foreground">
          Joined through an invite, as Plex account <strong>{viewer.plex.accountName}</strong> with the server{' '}
          <strong>{viewer.plex.serverName}</strong>. If they open their invite link again, the addresses below are replaced with that server's
          current ones.
        </p>
      )}

      <ViewerFields f={fields} devices={devices} />

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
