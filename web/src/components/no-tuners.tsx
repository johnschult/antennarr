import { Radar, Search } from 'lucide-react'
import { useState } from 'react'
import { AddTunerDialog } from '@/components/add-tuner-dialog'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'

/** What a page shows when no tuner has been added yet. */
export function NoTuners() {
  const { state } = useApp()
  const [open, setOpen] = useState(false)
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border px-6 py-12 text-center">
      <Radar className="size-8 text-muted-foreground" aria-hidden="true" />
      <h2 className="text-lg font-semibold tracking-tight">No tuner yet</h2>
      <p className="max-w-md text-sm text-muted-foreground">
        Yagi looks for HDHomeRun tuners on your network when it starts, and none answered. Search again, or add one by its address.
      </p>
      <Button onClick={() => setOpen(true)} disabled={!state.editable}>
        <Search data-icon="inline-start" /> Find tuners
      </Button>
      {!state.editable && <p className="text-xs text-muted-foreground">Yagi has no settings file to save a tuner in, so none can be added here. Start it with -config to choose one.</p>}
      <AddTunerDialog open={open} onOpenChange={setOpen} />
    </div>
  )
}
