import { PlexIcon } from '@/components/plex-button'
import type { ViewerPlex } from '@/lib/types'

/** "Plex: dad · Dad's server", for a viewer who enrolled through an invite: who signed in, and which server the addresses came from. */
export function PlexViewerNote({ plex }: { plex: ViewerPlex }) {
  return (
    <div className="mt-0.5 flex items-center gap-1.5 text-xs font-normal text-muted-foreground">
      <PlexIcon className="size-4" />
      <span className="min-w-0 truncate">
        Plex: {plex.accountName} · {plex.serverName}
      </span>
    </div>
  )
}
