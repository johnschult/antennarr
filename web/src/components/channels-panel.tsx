import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, EyeOff, ListChecks, Loader2, Radar, Square, Star } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ScanDialog } from '@/components/scan-dialog'
import { Section } from '@/components/settings/section'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useApp } from '@/hooks/app-context'
import { receptionByChannel } from '@/lib/activity'
import { getChannels, getHistory, setChannelState, startChannelScan, stopChannelScan } from '@/lib/api'
import { signalTone, toneText } from '@/lib/signal'
import type { ChannelsDoc, ChannelState, DeviceState, TunerChannel } from '@/lib/types'
import { cn } from '@/lib/utils'

type Filter = 'all' | ChannelState

const FILTERS: { id: Filter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'enabled', label: 'Enabled' },
  { id: 'favorite', label: 'Favorites' },
  { id: 'disabled', label: 'Disabled' },
]

const STATES: { id: ChannelState; label: string; icon: typeof Check }[] = [
  { id: 'enabled', label: 'Enabled', icon: Check },
  { id: 'favorite', label: 'Favorite', icon: Star },
  { id: 'disabled', label: 'Disabled', icon: EyeOff },
]

const SHOWN = 300

/** A channel number as a sort key: 2.1 before 2.10 before 11.1. */
const numberKey = (n: string) => n.split('.').map((p) => p.padStart(6, '0')).join('.')

/** The three states a channel can be in, as buttons: one is pressed. */
function StatePicker({ channel, onPick }: { channel: TunerChannel; onPick: (state: ChannelState) => void }) {
  return (
    <div role="group" aria-label={`Channel ${channel.number} ${channel.name}`} className="inline-flex shrink-0 overflow-hidden rounded-lg border border-border">
      {STATES.map((s) => {
        const on = channel.state === s.id
        return (
          <button
            key={s.id}
            type="button"
            aria-pressed={on}
            aria-label={s.label}
            title={s.label}
            disabled={!channel.subscribed}
            onClick={() => !on && onPick(s.id)}
            className={cn(
              'inline-flex size-8 items-center justify-center text-muted-foreground outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-40',
              !on && 'hover:bg-muted hover:text-foreground',
              on && s.id === 'enabled' && 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
              on && s.id === 'favorite' && 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
              on && s.id === 'disabled' && 'bg-red-500/15 text-red-700 dark:text-red-300',
            )}
          >
            <s.icon aria-hidden className={cn('size-4', on && s.id === 'favorite' && 'fill-current')} />
          </button>
        )
      })}
    </div>
  )
}

/** The scan: its status, and a button to start or stop it. */
function ScanCard({ doc, device, onStart, onStop, stopping }: { doc: ChannelsDoc; device: DeviceState; onStart: () => void; onStop: () => void; stopping: boolean }) {
  const s = doc.scan
  return (
    <div data-testid="scan" className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card/70 px-4 py-3">
      <div className="min-w-0 space-y-1.5">
        <div className="text-sm font-medium">Channel scan</div>
        {s.inProgress ? (
          <>
            <div role="status" className="text-xs text-muted-foreground">
              Scanning{s.source ? ` ${s.source}` : ''}: found {s.found ?? 0} programs ({s.progress ?? 0}%)
            </div>
            <div role="progressbar" aria-label="How far the scan has got" aria-valuemin={0} aria-valuemax={100} aria-valuenow={s.progress ?? 0} className="h-1.5 w-56 max-w-full overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${s.progress ?? 0}%` }} />
            </div>
          </>
        ) : (
          <div className="text-xs text-muted-foreground">
            {s.possible ? 'Look for channels again, after moving an antenna or when the broadcasters change.' : "The tuner can't scan for channels right now."}
          </div>
        )}
      </div>
      {s.inProgress ? (
        <Button size="sm" variant="outline" onClick={onStop} disabled={stopping}>
          {stopping ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Square data-icon="inline-start" />} Stop
        </Button>
      ) : (
        <Button size="sm" variant="outline" onClick={onStart} disabled={!s.possible || device.tuner.updating || device.tuner.restarting}>
          <Radar data-icon="inline-start" /> Scan for channels
        </Button>
      )}
    </div>
  )
}

/** A tuner's channels: which are enabled, favorites or disabled, and scanning for more. */
export function ChannelsPanel({ device }: { device: DeviceState }) {
  const qc = useQueryClient()
  const { state } = useApp()
  const key = ['channels', device.name] as const
  const online = device.tuner.online
  const q = useQuery({
    queryKey: key,
    queryFn: () => getChannels(device.name),
    enabled: online,
    staleTime: 10_000,
    retry: false,
    refetchOnWindowFocus: false,
    // while a scan runs, ask again for how far it has got
    refetchInterval: (query) => (query.state.data?.scan.inProgress ? 2000 : false),
  })
  // How good the reception has been on the channels that have been watched, from the activity history. It is only a
  // help: the list is the same without it, so a history that is off or can't be read leaves it out.
  const history = useQuery({ queryKey: ['history', 30], queryFn: () => getHistory(30), enabled: online && state.settings.history, staleTime: 60_000, retry: false, refetchOnWindowFocus: false })
  const reception = useMemo(() => receptionByChannel(history.data?.sessions ?? [], device.name), [history.data, device.name])
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const [scanning, setScanning] = useState(false)

  const change = useMutation({
    mutationFn: ({ number, state }: { number: string; state: ChannelState }) => setChannelState(device.name, number, state),
    onMutate: async ({ number, state }) => {
      await qc.cancelQueries({ queryKey: key })
      const before = qc.getQueryData<ChannelsDoc>(key)
      if (before) qc.setQueryData<ChannelsDoc>(key, { ...before, channels: before.channels.map((c) => (c.number === number ? { ...c, state } : c)) })
      return { before }
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.before) qc.setQueryData(key, ctx.before)
      toast.error(err instanceof Error ? err.message : 'Could not change the channel')
    },
  })
  const stop = useMutation({
    mutationFn: () => stopChannelScan(device.name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Could not stop the scan'),
  })
  const startScan = async (source: string, interrupt: boolean) => {
    try {
      await startChannelScan(device.name, source, interrupt)
      await qc.invalidateQueries({ queryKey: key })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Could not start the scan')
      throw err
    }
  }

  // One message when a scan that was running ends, and the channels it found are read.
  const wasScanning = useRef(false)
  const inProgress = q.data?.scan.inProgress ?? false
  useEffect(() => {
    if (wasScanning.current && !inProgress) {
      toast.success('Channel scan finished')
      void qc.invalidateQueries({ queryKey: key })
    }
    wasScanning.current = inProgress
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only when it starts or stops
  }, [inProgress])

  const channels = useMemo(() => [...(q.data?.channels ?? [])].sort((a, b) => numberKey(a.number).localeCompare(numberKey(b.number))), [q.data])
  const counts = useMemo(() => {
    const c = { enabled: 0, favorite: 0, disabled: 0 }
    for (const ch of channels) c[ch.state]++
    return c
  }, [channels])
  const shown = useMemo(() => {
    const term = search.trim().toLowerCase()
    return channels.filter((c) => (filter === 'all' || c.state === filter) && (term === '' || c.number.toLowerCase().includes(term) || c.name.toLowerCase().includes(term)))
  }, [channels, filter, search])

  return (
    <Section icon={ListChecks} title="Channels" description="Which channels viewers and media servers get. A disabled channel is left out of the lineup, and a favorite is starred. Where a channel has been watched, its average signal quality over the last 30 days is shown.">
      <div className="space-y-4 p-4">
        {!online ? (
          <p className="text-sm text-muted-foreground">The tuner is offline, so there is nothing to read.</p>
        ) : q.isPending ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 aria-hidden className="size-4 animate-spin" /> Asking the tuner…
          </p>
        ) : q.isError ? (
          <p role="alert" className="text-sm text-red-700 dark:text-red-300">
            {q.error instanceof Error ? q.error.message : "Couldn't read the tuner's channels."}
          </p>
        ) : (
          <>
            <ScanCard doc={q.data} device={device} onStart={() => setScanning(true)} onStop={() => stop.mutate()} stopping={stop.isPending} />

            <div className="flex flex-wrap items-center justify-between gap-3">
              <p data-testid="counts" className="text-sm text-muted-foreground">
                {counts.enabled + counts.favorite} in the lineup · {counts.favorite} {counts.favorite === 1 ? 'favorite' : 'favorites'} · {counts.disabled} disabled
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <div role="group" aria-label="Show" className="flex gap-1">
                  {FILTERS.map((f) => (
                    <Button key={f.id} size="sm" variant={filter === f.id ? 'default' : 'outline'} aria-pressed={filter === f.id} onClick={() => setFilter(f.id)}>
                      {f.label}
                    </Button>
                  ))}
                </div>
                <Input aria-label="Search channels" placeholder="Search" value={search} onChange={(e) => setSearch(e.target.value)} className="h-8 w-40" />
              </div>
            </div>

            {shown.length === 0 ? (
              <p className="rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-muted-foreground">
                {channels.length === 0 ? 'The tuner has not found any channels. Scan for them above.' : 'No channel matches.'}
              </p>
            ) : (
              <>
                <ul aria-label="Channels" className="divide-y divide-border overflow-hidden rounded-xl border border-border">
                  {shown.slice(0, SHOWN).map((c) => (
                    <li key={c.number} data-testid="channel" data-state={c.state} className="flex items-center gap-3 px-3 py-2 text-sm">
                      <span className="w-14 shrink-0 font-mono text-xs tabular-nums text-muted-foreground">{c.number}</span>
                      <span className={cn('min-w-0 flex-1 truncate', c.state === 'disabled' && 'text-muted-foreground line-through', !c.subscribed && 'text-muted-foreground')}>{c.name || '—'}</span>
                      {!c.subscribed && <span className="shrink-0 text-xs text-muted-foreground">not available to this tuner</span>}
                      {c.hd && <Badge variant="secondary">HD</Badge>}
                      {c.drm && <Badge variant="secondary">DRM</Badge>}
                      {c.video && <span className="hidden w-14 shrink-0 text-xs text-muted-foreground sm:block">{c.video}</span>}
                      {state.settings.history && (
                        <span className="hidden w-16 shrink-0 text-right text-xs tabular-nums sm:block" data-testid="reception">
                          {reception.has(c.number) ? (
                            <span className={toneText[signalTone(reception.get(c.number)!)]} title="The average signal quality while it was watched, over the last 30 days">
                              {reception.get(c.number)}%
                            </span>
                          ) : null}
                        </span>
                      )}
                      <StatePicker channel={c} onPick={(state) => change.mutate({ number: c.number, state })} />
                    </li>
                  ))}
                </ul>
                {shown.length > SHOWN && (
                  <p className="text-xs text-muted-foreground">
                    Showing the first {SHOWN} of {shown.length}: search to narrow them down.
                  </p>
                )}
              </>
            )}
          </>
        )}
      </div>
      <ScanDialog device={scanning ? device : null} scan={q.data?.scan} onClose={() => setScanning(false)} onStart={startScan} />
    </Section>
  )
}
