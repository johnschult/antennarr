import { Antenna, Cable, Info, Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { DeleteViewerDialog } from '@/components/delete-viewer-dialog'
import { ViewerDialog } from '@/components/viewer-dialog'
import { ViewerWizard } from '@/components/viewer-wizard/viewer-wizard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useApp } from '@/hooks/app-context'
import { useIsPhone } from '@/hooks/use-media-query'
import { timeAgo } from '@/lib/format'
import { deviceTitle } from '@/lib/device'
import { limitFor } from '@/lib/slots'
import type { DeviceState, Viewer } from '@/lib/types'
import { cn } from '@/lib/utils'

/** How many of a viewer's tuners are in use, per device, as antennas and a count. */
function Usage({ v, devices }: { v: Viewer; devices: DeviceState[] }) {
  const several = devices.length > 1
  return (
    <div className="space-y-1">
      {devices.map((d) => {
        const limit = limitFor(v, d.name)
        const inUse = v.inUseByDevice?.[d.name] ?? 0
        return (
          <div key={d.name} className="flex items-center gap-3">
            {several && <span className="w-16 truncate text-xs text-muted-foreground">{deviceTitle(d)}</span>}
            <div className="flex gap-1" aria-hidden="true">
              {Array.from({ length: limit }).map((_, i) => (
                <Antenna key={i} className={cn('size-4', i < inUse ? 'text-cyan-400' : 'text-muted-foreground/40')} />
              ))}
            </div>
            <span className="text-xs text-muted-foreground tabular-nums">
              {inUse}/{limit}
            </span>
          </div>
        )
      })}
    </div>
  )
}

/** Watching, paused, last seen, or still waiting for the viewer's first request. */
function ViewerStatus({ v, devices }: { v: Viewer; devices: DeviceState[] }) {
  if (v.inUse > 0) return <Badge className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">Watching</Badge>
  if (devices.every((d) => limitFor(v, d.name) === 0)) return <Badge variant="secondary">Paused</Badge>
  if (v.lastSeen) return <Badge variant="secondary">Seen {timeAgo(v.lastSeen)}</Badge>
  return <Badge className="bg-amber-500/15 text-amber-700 dark:text-amber-300">Waiting for first contact</Badge>
}

function RowActions({
  v,
  editable,
  onConnect,
  onEdit,
  onRemove,
}: {
  v: Viewer
  editable: boolean
  onConnect: (v: Viewer) => void
  onEdit: (v: Viewer) => void
  onRemove: (v: Viewer) => void
}) {
  return (
    <>
      <Button variant="ghost" size="icon" aria-label={`Connection steps for ${v.name}`} title="Connection steps" onClick={() => onConnect(v)}>
        <Cable />
      </Button>
      <Button variant="ghost" size="icon" aria-label={`Edit ${v.name}`} disabled={!editable} onClick={() => onEdit(v)}>
        <Pencil />
      </Button>
      <Button variant="ghost" size="icon" aria-label={`Remove ${v.name}`} disabled={!editable} onClick={() => onRemove(v)}>
        <Trash2 />
      </Button>
    </>
  )
}

export function ViewersPage() {
  const { state } = useApp()
  const { viewers, devices, editable } = state
  // A viewer's name goes to their page (what they watched and how much data it was) while the history is kept.
  const nameOf = (v: Viewer) =>
    state.settings.history ? (
      <Link to={`/viewers/${encodeURIComponent(v.name)}`} className="underline-offset-2 hover:underline">
        {v.name}
      </Link>
    ) : (
      v.name
    )
  const phone = useIsPhone()
  const [params, setParams] = useSearchParams()
  const [editing, setEditing] = useState<Viewer | undefined>()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<Viewer | null>(null)
  // The add-viewer wizard, or the connection steps for one viewer. A link with ?add opens it.
  const [wizard, setWizard] = useState<{ viewer?: Viewer } | null>(() => (params.has('add') && editable ? {} : null))

  useEffect(() => {
    if (params.has('add')) setParams({}, { replace: true })
  }, [params, setParams])

  function openAdd() {
    setWizard({})
  }
  function openConnect(v: Viewer) {
    setWizard({ viewer: v })
  }
  function openEdit(v: Viewer) {
    setEditing(v)
    setDialogOpen(true)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Viewers</h1>
          <p className="mt-1 text-sm text-muted-foreground">Who can use your tuner, from where, and how many tuners each gets.</p>
        </div>
        <Button onClick={openAdd} disabled={!editable}>
          <Plus /> Add viewer
        </Button>
      </div>

      {viewers.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
          <p className="font-medium">No viewers yet</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Add the person you share with. Only the addresses you list will be able to use the tuner.
          </p>
          <Button className="mt-4" onClick={openAdd} disabled={!editable}>
            <Plus /> Add viewer
          </Button>
        </div>
      ) : (
        phone ? (
          <ul aria-label="Viewers" className="space-y-3">
            {viewers.map((v) => (
              <li key={v.name} className="space-y-3 rounded-xl border border-border bg-card/70 p-4">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="truncate font-medium">{nameOf(v)}</div>
                    <div className="mt-0.5 font-mono text-xs break-all text-muted-foreground">{v.addresses.join(', ')}</div>
                  </div>
                  <div className="-mt-1 -mr-2 flex shrink-0">
                    <RowActions v={v} editable={editable} onConnect={openConnect} onEdit={openEdit} onRemove={setRemoving} />
                  </div>
                </div>
                <ViewerStatus v={v} devices={devices} />
                <Usage v={v} devices={devices} />
              </li>
            ))}
          </ul>
        ) : (
          <div className="rounded-xl border border-border bg-card/70">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-4">Viewer</TableHead>
                  <TableHead>Addresses</TableHead>
                  <TableHead>Tuners</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="w-32">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {viewers.map((v) => (
                  <TableRow key={v.name}>
                    <TableCell className="pl-4 font-medium">{nameOf(v)}</TableCell>
                    <TableCell className="font-mono text-xs">{v.addresses.join(', ')}</TableCell>
                    <TableCell>
                      <Usage v={v} devices={devices} />
                    </TableCell>
                    <TableCell>
                      <ViewerStatus v={v} devices={devices} />
                    </TableCell>
                    <TableCell className="text-right">
                      <RowActions v={v} editable={editable} onConnect={openConnect} onEdit={openEdit} onRemove={setRemoving} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )
      )}

      <div className="flex items-start gap-3 rounded-xl border border-sky-500/30 bg-sky-500/10 px-4 py-3 text-sm text-sky-900 dark:text-sky-200">
        <Info aria-hidden className="mt-0.5 size-4 shrink-0" />
        <p>
          <strong>Over a viewer's limit?</strong> Yagi answers with a 503, the same as a busy tuner does, so media servers handle it cleanly.
        </p>
      </div>

      <ViewerWizard open={wizard !== null} onOpenChange={(open) => !open && setWizard(null)} devices={devices} viewer={wizard?.viewer} />
      <ViewerDialog open={dialogOpen} onOpenChange={setDialogOpen} viewer={editing} devices={devices} />
      <DeleteViewerDialog viewer={removing} isLast={viewers.length === 1} onClose={() => setRemoving(null)} />
    </div>
  )
}
