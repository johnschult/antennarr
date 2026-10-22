import { Check, Copy, Link2, TriangleAlert, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { LimitFields } from '@/components/viewer-fields'
import { useEdits } from '@/hooks/mutations'
import { useInviteFields } from '@/hooks/use-invite-fields'
import { getNetwork } from '@/lib/api'
import { isLocalOrigin, linkChoices } from '@/lib/invite-link'
import type { DeviceState, InviteCreated } from '@/lib/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  devices: DeviceState[]
}

/** Inviting a viewer: who, and how many tuners, and then the link, which is shown only once. */
export function InviteDialog({ open, onOpenChange, devices }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
        {open && <InviteFlow devices={devices} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function InviteFlow({ devices, onDone }: { devices: DeviceState[]; onDone: () => void }) {
  const [created, setCreated] = useState<InviteCreated | null>(null)
  return created ? <LinkStep invite={created} onDone={onDone} /> : <FormStep devices={devices} onDone={onDone} onCreated={setCreated} />
}

function FormStep({ devices, onDone, onCreated }: { devices: DeviceState[]; onDone: () => void; onCreated: (i: InviteCreated) => void }) {
  const { inviteAdd } = useEdits()
  const fields = useInviteFields(devices)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const built = fields.build()
    if ('error' in built) return setError(built.error)
    try {
      onCreated(await inviteAdd.mutateAsync(built.input))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <DialogHeader>
        <DialogTitle>Invite a viewer with Plex</DialogTitle>
        <DialogDescription>
          Send them a link. They sign in with Plex and pick their Plex server, and Yagi allows its internet address, so you don't have to
          collect it.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="invite-name">Name</Label>
        <Input id="invite-name" value={fields.name} onChange={(e) => fields.setName(e.target.value)} placeholder="Mom & Dad" autoFocus />
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
        <Button type="submit" disabled={inviteAdd.isPending}>
          <Link2 data-icon="inline-start" /> Make the link
        </Button>
      </DialogFooter>
    </form>
  )
}

function LinkStep({ invite, onDone }: { invite: InviteCreated; onDone: () => void }) {
  const network = useQuery({ queryKey: ['network'], queryFn: getNetwork, staleTime: 60_000, retry: false, refetchOnWindowFocus: false })
  const here = { origin: window.location.origin, port: window.location.port, protocol: window.location.protocol }
  const choices = linkChoices(here, network.data?.addresses ?? [])
  const [chosen, setChosen] = useState<string | null>(null)
  const origin = choices.find((c) => c.origin === chosen)?.origin ?? choices[0].origin
  const link = origin + invite.path
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(link)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("Couldn't copy: select the link and copy it instead")
    }
  }

  return (
    <div className="space-y-4">
      <DialogHeader>
        <DialogTitle>Send this link to {invite.name}</DialogTitle>
        <DialogDescription>
          It is shown only this once: if you lose it, revoke the invite and make another. It works for 7 days. Once they have joined, it
          stays good so they can bring their address up to date, until you revoke it.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="invite-link">Invite link</Label>
        <div className="flex gap-2">
          <Input id="invite-link" readOnly value={link} className="font-mono text-xs" onFocus={(e) => e.currentTarget.select()} />
          <Button type="button" variant="outline" onClick={copy} aria-label="Copy the invite link">
            {copied ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </div>
      </div>

      {choices.length > 1 && (
        <div className="space-y-2">
          <Label htmlFor="invite-address">Address they will open it at</Label>
          <Select id="invite-address" value={origin} onChange={(e) => setChosen(e.target.value)}>
            {choices.map((c) => (
              <option key={c.origin} value={c.origin}>
                {c.label}
              </option>
            ))}
          </Select>
        </div>
      )}
      {isLocalOrigin(origin) && (
        <p role="alert" className="flex items-start gap-2 text-sm text-amber-700 dark:text-amber-300">
          <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
          <span>This link starts with an address that only works on this computer. After you copy it, replace the start with the address your viewer can reach, such as http://192.168.1.5:9244.</span>
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        They need to be able to open it: it is on the same port as this page, so it works wherever you let people reach that.
      </p>

      <DialogFooter>
        <Button type="button" onClick={onDone}>
          <Check data-icon="inline-start" /> Done
        </Button>
      </DialogFooter>
    </div>
  )
}
