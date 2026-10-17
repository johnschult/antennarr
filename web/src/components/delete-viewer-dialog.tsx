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
import type { Viewer } from '@/lib/types'

interface Props {
  viewer: Viewer | null
  /** True when this is the only viewer: removing it opens the tuner to everyone. */
  isLast: boolean
  onClose: () => void
}

export function DeleteViewerDialog({ viewer, isLast, onClose }: Props) {
  const { remove } = useEdits()
  const [error, setError] = useState<string | null>(null)

  async function confirm() {
    if (!viewer) return
    setError(null)
    try {
      await remove.mutateAsync({ name: viewer.name, allowOpen: isLast })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <AlertDialog open={viewer !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {viewer?.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            {isLast
              ? 'This is your only viewer. With no viewers, anyone who can reach this port can use your tuner.'
              : 'Their media server will no longer be able to use your tuner. A stream already running is not cut off.'}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>Keep</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={confirm} disabled={remove.isPending}>
            {isLast ? 'Remove and open access' : 'Remove'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
