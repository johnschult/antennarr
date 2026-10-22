import { Ban, Clock, UserCheck } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { timeUntil } from '@/lib/format'
import type { Invite } from '@/lib/types'

/** What state an invite is in, for the badge and the line under its name. */
function status(i: Invite): { badge: React.ReactNode; detail: string } {
  if (i.used) {
    return {
      badge: (
        <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">
          <UserCheck aria-hidden /> Joined
        </Badge>
      ),
      detail: `Joined as ${i.viewer ?? i.name}. The link stays good so they can update their address, until you revoke it.`,
    }
  }
  if (i.expired) {
    return {
      badge: (
        <Badge variant="secondary">
          <Clock aria-hidden /> Expired
        </Badge>
      ),
      detail: 'Nobody used it in time. Revoke it and make a new one.',
    }
  }
  return {
    badge: (
      <Badge className="bg-amber-500/15 text-amber-700 dark:text-amber-300">
        <Clock aria-hidden /> Waiting
      </Badge>
    ),
    detail: `Not used yet. It stops working ${timeUntil(i.expires)}.`,
  }
}

/** The invites that have been made: waiting for their person, joined, or run out. */
export function InvitesList({ invites, editable, onRevoke }: { invites: Invite[]; editable: boolean; onRevoke: (i: Invite) => void }) {
  if (invites.length === 0) return null
  return (
    <section aria-labelledby="invites-heading" className="space-y-2">
      <h2 id="invites-heading" className="text-sm font-semibold tracking-tight">
        Invites
      </h2>
      <ul aria-label="Invites" className="divide-y divide-border rounded-xl border border-border bg-card/70">
        {invites.map((i) => {
          const s = status(i)
          return (
            <li key={i.id} className="flex items-start justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-medium">{i.name}</span>
                  {s.badge}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">{s.detail}</p>
              </div>
              <Button variant="ghost" size="icon" aria-label={`Revoke the invite for ${i.name}`} disabled={!editable} onClick={() => onRevoke(i)}>
                <Ban />
              </Button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
