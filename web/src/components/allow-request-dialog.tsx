import { Check, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LimitFields } from '@/components/viewer-fields'
import { useEdits } from '@/hooks/mutations'
import { useAllowFields } from '@/hooks/use-allow-fields'
import type { AccessRequest, DeviceState } from '@/lib/types'

interface Props {
  request: AccessRequest | null
  devices: DeviceState[]
  onClose: () => void
}

/** How Yagi will recognize them, in words: by who the tailnet says they are, or by where they connect from. */
function recognized(r: AccessRequest): string {
  if (r.login) return `by their Tailscale login (${r.login}), from any of their devices`
  if (r.node) return `by that device (${r.node})`
  return `by their address (${r.address})`
}

/** Allowing someone who asked: a name and how many tuners they may use. They become a viewer. */
export function AllowRequestDialog({ request, devices, onClose }: Props) {
  return (
    <Dialog open={request !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>{request && <AllowForm key={request.id} request={request} devices={devices} onDone={onClose} />}</DialogContent>
    </Dialog>
  )
}

function AllowForm({ request, devices, onDone }: { request: AccessRequest; devices: DeviceState[]; onDone: () => void }) {
  const { requestAllow } = useEdits()
  const fields = useAllowFields(request, devices)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const built = fields.build()
    if ('error' in built) return setError(built.error)
    try {
      await requestAllow.mutateAsync({ id: request.id, input: built.input })
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <DialogHeader>
        <DialogTitle>Allow {request.label}</DialogTitle>
        <DialogDescription>They become a viewer, recognized {recognized(request)}.</DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="allow-name">Name</Label>
        <Input id="allow-name" value={fields.name} onChange={(e) => fields.setName(e.target.value)} placeholder="Mom & Dad" autoFocus />
      </div>
      <LimitFields f={fields} devices={devices} />

      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          <X data-icon="inline-start" /> Cancel
        </Button>
        <Button type="submit" disabled={requestAllow.isPending}>
          <Check data-icon="inline-start" /> Allow
        </Button>
      </DialogFooter>
    </form>
  )
}
