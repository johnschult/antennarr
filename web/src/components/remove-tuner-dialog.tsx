import { Trash2, Undo2 } from 'lucide-react'
import { useState } from 'react'
import {
  AlertDialog,
  AlertDialogAction,
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
  device: DeviceState | null
  onClose: () => void
}

export function RemoveTunerDialog({ device, onClose }: Props) {
  const { removeTuner } = useEdits()
  const [error, setError] = useState<string | null>(null)
  const watching = device?.streams.length ?? 0

  async function confirm() {
    if (!device) return
    setError(null)
    try {
      await removeTuner.mutateAsync({ name: device.name })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <AlertDialog open={device !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {device ? deviceTitle(device) : ''}?</AlertDialogTitle>
          <AlertDialogDescription>
            Yagi stops sharing this tuner, and its port is freed.
            {watching > 0 && ` ${watching === 1 ? 'One stream is' : `${watching} streams are`} running on it and will be cut off.`}{' '}
            The tuner itself is not touched, and you can add it again. Its name and any per-tuner limits for viewers are forgotten.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {error}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Keep
          </AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={confirm} disabled={removeTuner.isPending}>
            <Trash2 data-icon="inline-start" /> Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
