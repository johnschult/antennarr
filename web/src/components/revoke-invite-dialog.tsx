import { Ban, Undo2 } from 'lucide-react'
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
import type { Invite } from '@/lib/types'

interface Props {
  invite: Invite | null
  onClose: () => void
}

export function RevokeInviteDialog({ invite, onClose }: Props) {
  const { inviteRemove } = useEdits()
  const [error, setError] = useState<string | null>(null)

  async function confirm() {
    if (!invite) return
    setError(null)
    try {
      await inviteRemove.mutateAsync({ id: invite.id, name: invite.name })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <AlertDialog open={invite !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Revoke the invite for {invite?.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            {invite?.used
              ? `The link stops working. ${invite.viewer ?? 'The viewer'} stays a viewer, but can no longer bring their address up to date by opening it again.`
              : 'The link stops working. Anyone who has it, and has not used it yet, can no longer join with it.'}
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
          <AlertDialogAction variant="destructive" onClick={confirm} disabled={inviteRemove.isPending}>
            <Ban data-icon="inline-start" /> Revoke
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
