import { Plus, RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useEdits } from '@/hooks/mutations'
import { discoverTuners } from '@/lib/api'
import type { DeviceInput, FoundTuner } from '@/lib/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Find tuners on the network and add one, or add one by its address. */
export function AddTunerDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>{open && <AddTunerForm onDone={() => onOpenChange(false)} />}</DialogContent>
    </Dialog>
  )
}

const hostOf = (address: string) => address.replace(/^https?:\/\//, '')

function AddTunerForm({ onDone }: { onDone: () => void }) {
  const { addTuner } = useEdits()
  const search = useMutation({ mutationFn: discoverTuners })
  const [address, setAddress] = useState('')
  const [error, setError] = useState<string | null>(null)

  // Look as soon as the dialog opens; the person came here to add a tuner.
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    search.mutate()
  }, [search])

  async function add(input: DeviceInput) {
    setError(null)
    try {
      await addTuner.mutateAsync(input)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    const tuner = address.trim()
    if (!tuner) return setError("Enter the tuner's address, such as 192.168.1.50.")
    void add({ tuner })
  }

  const found: FoundTuner[] = search.data ?? []
  const busy = addTuner.isPending

  return (
    <div className="space-y-5">
      <DialogHeader>
        <DialogTitle>Add a tuner</DialogTitle>
        <DialogDescription>Antennarr shares the tuner with your viewers. It gets its own port.</DialogDescription>
      </DialogHeader>

      <section aria-label="Tuners on your network" className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">On your network</h3>
          <Button type="button" variant="ghost" size="sm" disabled={search.isPending} onClick={() => search.mutate()}>
            <RefreshCw data-icon="inline-start" /> Search again
          </Button>
        </div>
        {search.isPending && <p className="text-sm text-muted-foreground">Looking for tuners…</p>}
        {search.isError && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {search.error instanceof Error ? search.error.message : 'The search failed.'}
          </p>
        )}
        {search.isSuccess && found.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No tuner answered. If yours is on another network, or Antennarr runs in Docker without host networking, add it by its address below.
          </p>
        )}
        {found.length > 0 && (
          <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border">
            {found.map((f) => (
              <li key={f.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <div className="font-mono text-sm">{f.id}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {hostOf(f.address)} · {f.tuners} {f.tuners === 1 ? 'tuner' : 'tuners'}
                  </div>
                </div>
                {f.added ? (
                  <span className="shrink-0 text-xs text-muted-foreground">Already added as {f.added}</span>
                ) : (
                  <Button type="button" size="sm" disabled={busy} aria-label={`Add ${f.id}`} onClick={() => void add({ tuner: f.address, id: f.id })}>
                    <Plus data-icon="inline-start" /> Add
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      <form onSubmit={submit} noValidate className="space-y-2">
        <Label htmlFor="tuner-address">Or add one by its address</Label>
        <div className="flex gap-2">
          <Input id="tuner-address" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="192.168.1.50" spellCheck={false} className="font-mono" />
          <Button type="submit" disabled={busy}>
            <Plus data-icon="inline-start" /> Add
          </Button>
        </div>
      </form>

      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}
    </div>
  )
}
