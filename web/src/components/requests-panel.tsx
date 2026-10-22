import { Check, UserRoundPlus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useEdits } from '@/hooks/mutations'
import { deviceTitle } from '@/lib/device'
import { timeAgo } from '@/lib/format'
import type { AccessRequest, DeviceState } from '@/lib/types'

/** What the line under a requester says: how they were seen, how often and when, and which tuner. */
function detail(r: AccessRequest, devices: DeviceState[]): string {
  const parts: string[] = []
  if (r.login && r.node) parts.push(r.node)
  else if (r.login || r.node) parts.push(r.address)
  parts.push(r.count === 1 ? 'asked once' : `asked ${r.count} times`, timeAgo(r.last))
  const d = devices.find((x) => x.name === r.device)
  if (d && devices.length > 1) parts.push(`for ${deviceTitle(d)}`)
  return parts.join(' · ')
}

/**
 * People who tried to use a tuner and aren't viewers yet. Yagi serves private networks only, so these are people on
 * your network, a VPN or your tailnet: a friend who has just added Yagi in their media server. Allow makes them a
 * viewer.
 */
export function RequestsPanel({ requests, devices, editable, onAllow }: { requests: AccessRequest[]; devices: DeviceState[]; editable: boolean; onAllow: (r: AccessRequest) => void }) {
  const { requestDismiss } = useEdits()
  if (requests.length === 0) return null
  return (
    <section aria-labelledby="requests-heading" className="space-y-2">
      <div>
        <h2 id="requests-heading" className="flex items-center gap-2 text-sm font-semibold tracking-tight">
          <UserRoundPlus aria-hidden className="size-4 text-amber-600 dark:text-amber-300" /> Asking to use your tuner
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground">These people tried to connect and aren't viewers yet. Allow the ones you know.</p>
      </div>
      <ul aria-label="Requests" className="divide-y divide-border rounded-xl border border-amber-500/30 bg-amber-500/5">
        {requests.map((r) => (
          <li key={r.id} className="flex items-center justify-between gap-3 px-4 py-3">
            <div className="min-w-0">
              <div className="truncate font-medium">{r.label}</div>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">{detail(r, devices)}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Button size="sm" onClick={() => onAllow(r)} disabled={!editable} aria-label={`Allow ${r.label}`}>
                <Check data-icon="inline-start" /> Allow
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Dismiss ${r.label}`}
                title="Dismiss"
                disabled={!editable || requestDismiss.isPending}
                onClick={() => requestDismiss.mutate({ id: r.id, label: r.label })}
              >
                <X />
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
