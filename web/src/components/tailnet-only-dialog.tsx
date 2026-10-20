import { Lock, Undo2 } from 'lucide-react'
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
import type { TailscaleStatus } from '@/lib/types'

interface Props {
  open: boolean
  onClose: () => void
  status: TailscaleStatus
}

/** Whether this page was opened over the tailnet, or from the machine itself: either way it keeps working. */
export function reachedAnotherWay(status: TailscaleStatus, host = window.location.hostname): boolean {
  const here = host.toLowerCase()
  if (here === 'localhost' || here === '127.0.0.1' || here === '[::1]' || here === '::1') return false
  return !(here === status.dnsName?.toLowerCase() || status.addresses?.includes(here))
}

/** Asks before closing Yagi to the LAN, saying what will stop and how to get back. */
export function TailnetOnlyDialog({ open, onClose, status }: Props) {
  const { settings } = useEdits()
  const canAdmin = status.adminReachable !== false // the server knows the address family and ranges
  const port = window.location.port
  const where = status.dnsName ? `${status.dnsName.replace(/\.$/, '')}${port ? `:${port}` : ''}` : 'the tailnet name'

  async function confirm() {
    try {
      await settings.mutateAsync({ tailnetOnly: true })
      onClose()
    } catch {
      // the mutation has already said why in a toast; leave the dialog open to try again or cancel
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Only answer on the tailnet?</AlertDialogTitle>
          <AlertDialogDescription>
            Yagi will answer on the tailnet and to this machine itself. Anything that comes in over your network&apos;s own address is refused: media servers
            and viewers on the LAN, and this page opened at the LAN address. A stream running now keeps going until it ends.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {reachedAnotherWay(status) && (
          <p role="alert" className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm">
            You are using this page over the LAN address, so it will stop working. Open it at <span className="font-mono">{where}</span> afterwards.
          </p>
        )}
        {!canAdmin && (
          <p role="alert" className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm">
            Nobody on the tailnet is allowed to open this page yet, so only this machine could. Add <span className="font-mono">100.64.0.0/10</span> (your whole tailnet) or your own tailnet address to Admin addresses, or turn on sign-in with Plex, first.
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          To get back in, switch this off from the page over the tailnet, or start Yagi with <code>-allow-lan</code> (<code>YAGI_ALLOW_LAN=1</code> in Docker).
        </p>
        <AlertDialogFooter>
          <AlertDialogCancel>
            <Undo2 data-icon="inline-start" /> Keep the LAN open
          </AlertDialogCancel>
          <AlertDialogAction onClick={confirm} disabled={settings.isPending}>
            <Lock data-icon="inline-start" /> Close the LAN
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
