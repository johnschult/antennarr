import { AlertTriangle, RotateCw, Undo2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useEdits } from '@/hooks/mutations'
import { deviceTitle } from '@/lib/device'
import type { DeviceState } from '@/lib/types'

interface Props {
  /** The tuner to restart; null when the dialog is closed. */
  device: DeviceState | null
  onClose: () => void
}

/**
 * Asks before restarting a tuner, since it cuts off whatever is running. Whether the person has agreed to
 * interrupt what is in use is sent with the answer; the server refuses without it.
 */
export function RestartDialog({ device, onClose }: Props) {
  const { restart } = useEdits()
  const inUse = device?.summary.inUse ?? 0
  const title = device ? deviceTitle(device) : ''

  async function confirm() {
    if (!device) return
    try {
      await restart.mutateAsync({ device: device.name, interrupt: inUse > 0 })
      onClose()
    } catch {
      // the mutation has already said why in a toast; leave the dialog open to try again or cancel
    }
  }

  return (
    <AlertDialog open={device !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Restart {title}?</AlertDialogTitle>
          <AlertDialogDescription>
            The tuner turns itself off and on again. It is out of reach for a short while and comes back on its own, so there is nothing to unplug.
          </AlertDialogDescription>
        </AlertDialogHeader>

        {inUse > 0 && (
          <div role="alert" className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-200">
            <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0" />
            <span>
              <strong>
                {inUse} of its {inUse === 1 ? 'tuner is' : 'tuners are'} in use right now.
              </strong>{' '}
              That is viewers, your own devices or recordings, and restarting cuts them off.
            </span>
          </div>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Not now
          </AlertDialogCancel>
          <Button variant={inUse > 0 ? 'destructive' : 'default'} onClick={confirm} disabled={restart.isPending}>
            <RotateCw data-icon="inline-start" /> {inUse > 0 ? 'Interrupt and restart' : 'Restart'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
