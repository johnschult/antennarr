import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { DeleteViewerDialog } from '@/components/delete-viewer-dialog'
import { ViewerDialog } from '@/components/viewer-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useApp } from '@/hooks/app-context'
import { timeAgo } from '@/lib/format'
import type { Viewer } from '@/lib/types'
import { cn } from '@/lib/utils'

export function ViewersPage() {
  const { state } = useApp()
  const { viewers, tuner, editable } = state
  const [editing, setEditing] = useState<Viewer | undefined>()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [removing, setRemoving] = useState<Viewer | null>(null)

  function openAdd() {
    setEditing(undefined)
    setDialogOpen(true)
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
          <p className="mt-1 text-sm text-zinc-400">Who can use your tuner, from where, and how many tuners each gets.</p>
        </div>
        <Button onClick={openAdd} disabled={!editable}>
          <Plus /> Add viewer
        </Button>
      </div>

      {viewers.length === 0 ? (
        <div className="rounded-xl border border-dashed border-zinc-800 px-6 py-12 text-center">
          <p className="font-medium">No viewers yet</p>
          <p className="mt-1 text-sm text-zinc-400">
            Add the person you share with. Only the addresses you list will be able to use the tuner.
          </p>
        </div>
      ) : (
        <div className="rounded-xl border border-zinc-800 bg-zinc-900/50">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Viewer</TableHead>
                <TableHead>Addresses</TableHead>
                <TableHead>Tuners</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {viewers.map((v) => (
                <TableRow key={v.name}>
                  <TableCell className="font-medium">{v.name}</TableCell>
                  <TableCell className="font-mono text-xs">{v.addresses.join(', ')}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <div className="flex gap-1" aria-hidden="true">
                        {Array.from({ length: v.limit }).map((_, i) => (
                          <div key={i} className={cn('h-4 w-2 rounded-sm', i < v.inUse ? 'bg-cyan-400' : 'bg-zinc-800')} />
                        ))}
                      </div>
                      <span className="text-xs text-zinc-400 tabular-nums">
                        {v.inUse}/{v.limit}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    {v.inUse > 0 ? (
                      <Badge className="bg-emerald-500/15 text-emerald-300">Watching</Badge>
                    ) : v.limit === 0 ? (
                      <Badge variant="secondary">Paused</Badge>
                    ) : v.lastSeen ? (
                      <Badge variant="secondary">Seen {timeAgo(v.lastSeen)}</Badge>
                    ) : (
                      <Badge className="bg-amber-500/15 text-amber-300">Waiting for first contact</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="icon" aria-label={`Edit ${v.name}`} disabled={!editable} onClick={() => openEdit(v)}>
                      <Pencil />
                    </Button>
                    <Button variant="ghost" size="icon" aria-label={`Remove ${v.name}`} disabled={!editable} onClick={() => setRemoving(v)}>
                      <Trash2 />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <p className="text-xs text-zinc-500">
        Over-limit requests get a 503, the same answer a busy tuner gives, so media servers handle it cleanly.
      </p>

      <ViewerDialog open={dialogOpen} onOpenChange={setDialogOpen} viewer={editing} tunerCount={tuner.tunerCount} />
      <DeleteViewerDialog viewer={removing} isLast={viewers.length === 1} onClose={() => setRemoving(null)} />
    </div>
  )
}
