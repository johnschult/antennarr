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
import type { NotifyChannel } from '@/lib/types'

interface Props {
  channel: NotifyChannel | null
  onClose: () => void
}

export function RemoveChannelDialog({ channel, onClose }: Props) {
  const { channelRemove } = useEdits()
  const [error, setError] = useState<string | null>(null)

  async function confirm() {
    if (!channel) return
    setError(null)
    try {
      await channelRemove.mutateAsync({ id: channel.id, name: channel.name })
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <AlertDialog open={channel !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {channel?.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Antennarr stops sending messages there, and forgets its saved token and keys. Adding it again means entering them again.
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
          <AlertDialogAction variant="destructive" onClick={confirm} disabled={channelRemove.isPending}>
            <Trash2 data-icon="inline-start" /> Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
