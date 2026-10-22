import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { PlexFlowMessage, PlexIcon } from '@/components/plex-button'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { STATE_KEY } from '@/hooks/use-live-state'
import { usePlexFlow } from '@/hooks/use-plex-flow'
import { removePlexAdmin } from '@/lib/api'
import { toast } from 'sonner'
import { Row } from './section'
import { SwitchRow } from './switch-row'

/**
 * The Plex section of Access: two independent things Yagi can use plex.tv for. Signing in to this page with a Plex
 * account (and who may), and letting viewers join through an invite link. Neither needs the other, and nothing
 * contacts plex.tv until someone uses one.
 */
export function PlexRows() {
  const { state } = useApp()
  const { plex } = state.settings
  const { editable } = state
  const { settings: update } = useEdits()

  return (
    <>
      <SwitchRow
        id="setting-plex"
        title="Sign in with Plex"
        icon={<PlexIcon className="size-8" />}
        description="Lets the Plex accounts you choose open this page from anywhere Yagi can be reached, without listing their addresses. Nothing contacts plex.tv until someone signs in."
        checked={plex.enabled}
        disabled={!editable}
        onChange={(next) => update.mutateAsync({ plexEnabled: next })}
      />
      {plex.enabled && <PlexAccounts />}
      <SwitchRow
        id="setting-plex-enrollment"
        title="Invite viewers with Plex"
        icon={<PlexIcon className="size-8" />}
        description="Adds Invite with Plex to the Viewers page. You send someone a link, they sign in with Plex and pick their Plex server, and Yagi allows that server's internet address with the limit you set, so you don't collect addresses by hand. Their Plex password and sign-in are not kept. The link opens a public page on this port. Nothing contacts plex.tv until someone opens a link."
        checked={plex.enrollment}
        disabled={!editable}
        onChange={(next) => update.mutateAsync({ plexEnrollment: next })}
      />
    </>
  )
}

function PlexAccounts() {
  const { state } = useApp()
  const { admins } = state.settings.plex
  const { editable } = state
  const qc = useQueryClient()
  const [removing, setRemoving] = useState<number | null>(null)
  const { flow, start, cancel } = usePlexFlow({ add: true, onDone: () => void qc.invalidateQueries({ queryKey: STATE_KEY }) })
  const busy = flow.phase === 'starting' || flow.phase === 'waiting'

  async function remove(id: number, name: string) {
    setRemoving(id)
    try {
      await removePlexAdmin(id)
      toast.success(`${name} can no longer sign in`)
      void qc.invalidateQueries({ queryKey: STATE_KEY })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Could not remove that account')
    } finally {
      setRemoving(null)
    }
  }

  return (
    <Row
      title="Allowed Plex accounts"
      description="Only these accounts can sign in. Removing one signs it out straight away. This page is still open from the admin addresses above either way."
    >
      <div className="flex w-full min-w-0 flex-col items-stretch gap-2 sm:w-72">
        {admins.length === 0 && <p className="text-xs text-muted-foreground">No accounts yet.</p>}
        <ul className="space-y-1.5">
          {admins.map((a) => (
            <li key={a.id} className="flex items-center gap-2 rounded-lg border border-border bg-background/60 px-2.5 py-1.5 text-sm">
              <PlexIcon className="size-5" />
              <span className="min-w-0 flex-1 truncate">{a.name}</span>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Remove ${a.name}`}
                disabled={!editable || removing === a.id}
                onClick={() => remove(a.id, a.name)}
              >
                <Trash2 />
              </Button>
            </li>
          ))}
        </ul>
        <Button size="sm" variant="outline" onClick={start} disabled={!editable || busy}>
          {busy ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Plus data-icon="inline-start" />}
          {busy ? 'Waiting for Plex…' : 'Add a Plex account'}
        </Button>
        <PlexFlowMessage flow={flow} onCancel={cancel} adding />
      </div>
    </Row>
  )
}
