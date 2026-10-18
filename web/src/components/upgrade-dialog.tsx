import { AlertTriangle, Download, Undo2 } from 'lucide-react'
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
import { firmwareDate } from '@/lib/format'
import type { DeviceState } from '@/lib/types'

interface Props {
  /** The tuner to update; null when the dialog is closed. */
  device: DeviceState | null
  onClose: () => void
}

/**
 * Asks before installing firmware, since it can't be undone from here and it cuts off whatever is running.
 * What the server needs to proceed is sent with the answer: the release being offered (so a newer one found
 * since is not installed unseen) and whether the person has agreed to interrupt what is in use.
 */
export function UpgradeDialog({ device, onClose }: Props) {
  const { install } = useEdits()
  const release = device?.firmware.latest ?? ''
  const inUse = device?.summary.inUse ?? 0
  const title = device ? deviceTitle(device) : ''

  async function confirm() {
    if (!device) return
    try {
      await install.mutateAsync({ device: device.name, request: { version: release, interrupt: inUse > 0 } })
      onClose()
    } catch {
      // the mutation has already said why in a toast; leave the dialog open to try again or cancel
    }
  }

  return (
    <AlertDialog open={device !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Update firmware to {firmwareDate(release)}?</AlertDialogTitle>
          <AlertDialogDescription>
            Antennarr will download this release from SiliconDust and install it on {title}. The tuner restarts, which takes two to three minutes. Don't unplug it while it updates.
          </AlertDialogDescription>
        </AlertDialogHeader>

        {inUse > 0 && (
          <div role="alert" className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-200">
            <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0" />
            <span>
              <strong>
                {inUse} of its {inUse === 1 ? 'tuner is' : 'tuners are'} in use right now.
              </strong>{' '}
              That is viewers, your own devices or recordings, and updating cuts them off.
            </span>
          </div>
        )}

        <p className="text-xs text-muted-foreground">
          Downloaded from download.silicondust.com over HTTPS. Only the latest release for this model is offered, and it can't be undone from here.
        </p>

        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Not now
          </AlertDialogCancel>
          <Button variant={inUse > 0 ? 'destructive' : 'default'} onClick={confirm} disabled={install.isPending}>
            <Download data-icon="inline-start" /> {inUse > 0 ? 'Interrupt and update' : 'Update'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
