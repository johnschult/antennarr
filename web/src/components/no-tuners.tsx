import { Loader2, Radar, RefreshCw, Plus } from 'lucide-react'
import { useState } from 'react'
import { AddTunerDialog } from '@/components/add-tuner-dialog'
import { FoundTuners } from '@/components/found-tuners'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { useFindTuners } from '@/hooks/use-find-tuners'
import type { DeviceInput } from '@/lib/types'

/**
 * What a page shows when no tuner has been added yet. Yagi looks for tuners while this is open and lists what it
 * finds, with a name filled in; the person adds them. Nothing is added by itself.
 */
export function NoTuners() {
  const { state } = useApp()
  const { addTuner } = useEdits()
  const { found, looking, error, lookAgain } = useFindTuners()
  const [open, setOpen] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)

  async function add(inputs: DeviceInput[]) {
    setAddError(null)
    try {
      for (const input of inputs) await addTuner.mutateAsync(input)
    } catch (err) {
      setAddError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  const seen = found.length > 0
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border px-6 py-10 text-center">
      <Radar className="size-8 text-muted-foreground" aria-hidden="true" />
      <h2 className="text-lg font-semibold tracking-tight">{seen ? 'Tuners Yagi can see' : 'No tuner yet'}</h2>
      {!seen && (
        <p className="flex max-w-md items-center justify-center gap-2 text-sm text-muted-foreground" role="status">
          {looking ? (
            <>
              <Loader2 aria-hidden className="size-4 shrink-0 animate-spin" /> Looking for HDHomeRun tuners on your network…
            </>
          ) : (
            'Yagi looked for a few minutes and no tuner answered. Look again, or add one by its address.'
          )}
        </p>
      )}
      {seen && <p className="max-w-md text-sm text-muted-foreground">Choose a name for each, then add it. Yagi shares it with your viewers on its own port.</p>}
      {seen && state.editable && (
        <div className="w-full max-w-xl text-left">
          <FoundTuners tuners={found} busy={addTuner.isPending} onAdd={(inputs) => void add(inputs)} />
        </div>
      )}
      {error && !seen && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}
      {addError && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {addError}
        </p>
      )}
      <div className="flex flex-wrap justify-center gap-2">
        <Button variant="outline" onClick={lookAgain} disabled={looking}>
          {looking ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <RefreshCw data-icon="inline-start" />} Look again
        </Button>
        <Button variant="outline" onClick={() => setOpen(true)} disabled={!state.editable}>
          <Plus data-icon="inline-start" /> Add by address
        </Button>
      </div>
      {!seen && !looking && (
        <p className="max-w-md text-xs text-muted-foreground">
          Yagi in Docker can't see tuners unless the container uses host networking (<code>network_mode: host</code>). Otherwise add the tuner by its address, or set <code>YAGI_TUNER</code>.
        </p>
      )}
      {!state.editable && <p className="text-xs text-muted-foreground">Yagi has no settings file to save a tuner in, so none can be added here. Start it with -config to choose one.</p>}
      <AddTunerDialog open={open} onOpenChange={setOpen} />
    </div>
  )
}
