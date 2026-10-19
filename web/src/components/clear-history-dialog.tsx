import { Trash2, Undo2 } from 'lucide-react'
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

interface Props {
  open: boolean
  onClose: () => void
}

/** Asks before forgetting the activity history, since it can't be brought back. */
export function ClearHistoryDialog({ open, onClose }: Props) {
  const { history } = useEdits()

  async function confirm() {
    try {
      await history.mutateAsync()
      onClose()
    } catch {
      // the mutation has already said why in a toast; leave the dialog open to try again or cancel
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Clear the activity history?</AlertDialogTitle>
          <AlertDialogDescription>
            Yagi forgets who watched what, for every day it has kept. It can't be brought back. If the history is on, it starts again from now.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Keep it
          </AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={confirm} disabled={history.isPending}>
            <Trash2 data-icon="inline-start" /> Clear
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
