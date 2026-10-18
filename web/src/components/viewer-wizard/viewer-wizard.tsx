import { Check, ChevronLeft, ChevronRight, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ViewerFields } from '@/components/viewer-fields'
import { useEdits } from '@/hooks/mutations'
import { useViewerFields } from '@/hooks/use-viewer-fields'
import type { DeviceState, Viewer } from '@/lib/types'
import { cn } from '@/lib/utils'
import { ConnectStep } from './connect-step'
import { MediaServerStep } from './media-server-step'
import { useConnectGuide } from './use-connect-guide'

type Step = 'Who' | 'Connect' | 'Media server'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  devices: DeviceState[]
  /** An existing viewer to show the connection steps for. Without one, the wizard starts by adding a viewer. */
  viewer?: Viewer
}

/** Adding a viewer, then how to connect them: who, how their server reaches this one, and what to do in their media server. */
export function ViewerWizard({ open, onOpenChange, devices, viewer }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl">
        {open && <Wizard key={viewer?.name ?? '+'} devices={devices} viewer={viewer} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function Stepper({ steps, index }: { steps: Step[]; index: number }) {
  return (
    <ol aria-label="Steps" className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      {steps.map((s, i) => (
        <li key={s} aria-current={i === index ? 'step' : undefined} className={cn('flex items-center gap-2', i === index ? 'font-semibold text-foreground' : 'text-muted-foreground')}>
          <span
            aria-hidden
            className={cn('flex size-5 items-center justify-center rounded-full border text-[11px]', i === index ? 'border-primary bg-primary/15' : i < index ? 'border-emerald-500/60 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300' : 'border-border')}
          >
            {i < index ? <Check className="size-3" /> : i + 1}
          </span>
          {s}
          {i < steps.length - 1 && <span aria-hidden className="mx-1 h-px w-4 bg-border" />}
        </li>
      ))}
    </ol>
  )
}

function Wizard({ devices, viewer, onDone }: { devices: DeviceState[]; viewer?: Viewer; onDone: () => void }) {
  const adding = !viewer
  const steps: Step[] = adding ? ['Who', 'Connect', 'Media server'] : ['Connect', 'Media server']
  const [index, setIndex] = useState(0)
  // Once the viewer is added they exist for good, so it is their saved addresses that the later steps use.
  const [added, setAdded] = useState<{ name: string; addresses: string[] } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { add } = useEdits()
  const fields = useViewerFields(undefined, devices)

  const current = steps[index]
  const who = viewer ?? added
  const guide = useConnectGuide(who?.addresses ?? [], current !== 'Who')
  const last = index === steps.length - 1
  // Going back to "Who" after the viewer is saved would add them twice.
  const canBack = index > (adding ? 1 : 0)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (current !== 'Who') return
    setError(null)
    const built = fields.build()
    if ('error' in built) return setError(built.error)
    try {
      await add.mutateAsync(built.input)
      setAdded({ name: built.input.name, addresses: built.input.addresses })
      setIndex(1)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  const description =
    current === 'Who'
      ? "A viewer is someone's media server. It is recognized by the address it connects from."
      : current === 'Connect'
        ? `How ${who?.name ?? 'the viewer'}'s media server will reach this machine.`
        : `What to do in ${who?.name ?? 'the viewer'}'s media server.`

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <DialogHeader>
        <DialogTitle>{adding ? 'Add a viewer' : `Connect ${viewer.name}`}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <Stepper steps={steps} index={index} />

      {current === 'Who' && (
        <>
          <ViewerFields f={fields} devices={devices} />
          <p className="text-xs text-muted-foreground">The viewer is saved when you continue, so you can close the dialog at any point after that.</p>
        </>
      )}
      {current === 'Connect' && <ConnectStep g={guide} />}
      {current === 'Media server' && <MediaServerStep g={guide} />}

      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}

      <DialogFooter>
        {canBack ? (
          <Button type="button" variant="outline" onClick={() => setIndex(index - 1)}>
            <ChevronLeft data-icon="inline-start" /> Back
          </Button>
        ) : (
          <Button type="button" variant="outline" onClick={onDone}>
            <X data-icon="inline-start" /> {adding && !added ? 'Cancel' : 'Close'}
          </Button>
        )}
        {current === 'Who' ? (
          <Button type="submit" disabled={add.isPending}>
            Next: {steps[index + 1]} <ChevronRight data-icon="inline-end" />
          </Button>
        ) : last ? (
          <Button type="button" onClick={onDone}>
            <Check data-icon="inline-start" /> Done
          </Button>
        ) : (
          <Button type="button" onClick={() => setIndex(index + 1)}>
            Next: {steps[index + 1]} <ChevronRight data-icon="inline-end" />
          </Button>
        )}
      </DialogFooter>
    </form>
  )
}
