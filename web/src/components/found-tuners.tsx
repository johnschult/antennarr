import { Plus } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { DeviceInput, FoundTuner } from '@/lib/types'

const hostOf = (address: string) => address.replace(/^https?:\/\//, '')

interface Props {
  tuners: FoundTuner[]
  busy: boolean
  /** Add these, one request each, in order. Resolves when done; the caller shows any refusal. */
  onAdd: (inputs: DeviceInput[]) => void
}

/**
 * The tuners Yagi can see, each with a display name and a device name filled in for the person to change, and an
 * Add. Nothing here is added until one of the buttons is pressed.
 */
export function FoundTuners({ tuners, busy, onAdd }: Props) {
  // What the person has typed, by DeviceID. Until they type, the suggestion shows.
  const [typed, setTyped] = useState<Record<string, { label?: string; name?: string }>>({})
  const set = (id: string, v: { label?: string; name?: string }) => setTyped((t) => ({ ...t, [id]: { ...t[id], ...v } }))
  const inputFor = (f: FoundTuner): DeviceInput => {
    const label = (typed[f.id]?.label ?? f.label).trim()
    const name = (typed[f.id]?.name ?? f.name).trim()
    return { tuner: f.address, id: f.id, ...(name && { name }), ...(label && { label }) }
  }
  const addable = tuners.filter((f) => !f.added)

  return (
    <div className="space-y-2">
      <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border">
        {tuners.map((f) => (
          <li key={f.id} className="space-y-2 px-3 py-2.5">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="font-mono text-sm">{f.id}</div>
                <div className="truncate text-xs text-muted-foreground">
                  {hostOf(f.address)} · {f.tuners} {f.tuners === 1 ? 'tuner' : 'tuners'}
                </div>
              </div>
              {f.added ? (
                <span className="shrink-0 text-xs text-muted-foreground">Already added as {f.added}</span>
              ) : (
                <Button type="button" size="sm" disabled={busy} aria-label={`Add ${f.id}`} onClick={() => onAdd([inputFor(f)])}>
                  <Plus data-icon="inline-start" /> Add
                </Button>
              )}
            </div>
            {!f.added && (
              <div className="grid gap-2 sm:grid-cols-2">
                <label className="space-y-1 text-xs text-muted-foreground">
                  Name
                  <Input
                    value={typed[f.id]?.label ?? f.label}
                    onChange={(e) => set(f.id, { label: e.target.value })}
                    aria-label={`Name for ${f.id}`}
                    placeholder="Living room antenna"
                    className="text-sm text-foreground"
                  />
                </label>
                <label className="space-y-1 text-xs text-muted-foreground">
                  Address name
                  <Input
                    value={typed[f.id]?.name ?? f.name}
                    onChange={(e) => set(f.id, { name: e.target.value })}
                    aria-label={`Address name for ${f.id}`}
                    spellCheck={false}
                    className="font-mono text-sm text-foreground"
                  />
                </label>
              </div>
            )}
          </li>
        ))}
      </ul>
      {addable.length > 1 && (
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onAdd(addable.map(inputFor))}>
          <Plus data-icon="inline-start" /> Add all {addable.length}
        </Button>
      )}
    </div>
  )
}
