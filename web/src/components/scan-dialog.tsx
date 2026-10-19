import { AlertTriangle, Radar, Undo2 } from 'lucide-react'
import { useState } from 'react'
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
import { deviceTitle } from '@/lib/device'
import type { ChannelScan, DeviceState } from '@/lib/types'

interface Props {
  /** The tuner to scan; null when the dialog is closed. */
  device: DeviceState | null
  scan: ChannelScan | undefined
  onClose: () => void
  onStart: (source: string, interrupt: boolean) => Promise<unknown>
}

/**
 * Asks before scanning for channels, since a scan takes all of the tuner's tuners for as long as it runs and
 * cuts off whatever is using them. What the server needs to go ahead is sent with the answer: the source, and
 * whether the person has agreed to interrupt what is in use.
 */
export function ScanDialog({ device, scan, onClose, onStart }: Props) {
  const inUse = device?.summary.inUse ?? 0
  const sources = scan?.sources ?? []
  const [chosen, setChosen] = useState<string>()
  const source = chosen && sources.includes(chosen) ? chosen : (scan?.source ?? sources[0] ?? '')
  const [busy, setBusy] = useState(false)

  async function confirm() {
    setBusy(true)
    try {
      await onStart(source, inUse > 0)
      onClose()
    } catch {
      // the caller has already said why in a toast; leave the dialog open to try again or cancel
    } finally {
      setBusy(false)
    }
  }

  return (
    <AlertDialog open={device !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Scan {device ? deviceTitle(device) : ''} for channels?</AlertDialogTitle>
          <AlertDialogDescription>
            The tuner looks for channels again, which can take a while. It uses all of its tuners until it is done.
          </AlertDialogDescription>
        </AlertDialogHeader>

        {sources.length > 1 && (
          <div className="space-y-1.5">
            <label htmlFor="scan-source" className="text-sm font-medium">
              Scan
            </label>
            <select
              id="scan-source"
              value={source}
              onChange={(e) => setChosen(e.target.value)}
              className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              {sources.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
        )}

        {inUse > 0 && (
          <div role="alert" className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-200">
            <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0" />
            <span>
              <strong>
                {inUse} of its {inUse === 1 ? 'tuner is' : 'tuners are'} in use right now.
              </strong>{' '}
              That is viewers, your own devices or recordings, and a scan cuts them off.
            </span>
          </div>
        )}

        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Not now
          </AlertDialogCancel>
          <Button variant={inUse > 0 ? 'destructive' : 'default'} onClick={confirm} disabled={busy}>
            <Radar data-icon="inline-start" /> {inUse > 0 ? 'Interrupt and scan' : 'Scan'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
