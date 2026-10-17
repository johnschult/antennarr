import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'

/** The port from a listen address like ":5004" or "127.0.0.1:5004". */
const portOf = (listen: string) => listen.slice(listen.lastIndexOf(':') + 1)

/** Placeholder: the generated setup commands are the next milestone. */
export function SetupPage() {
  const { state } = useApp()
  const host = window.location.hostname
  const several = state.devices.length > 1
  return (
    <div className="max-w-2xl space-y-4">
      <h1 className="text-xl font-semibold tracking-tight">Setup guide</h1>
      <p className="text-sm text-muted-foreground">
        Copy-paste commands for each viewer's firewall, routing and media server are coming next. For now, a viewer adds{' '}
        {several ? 'each tuner' : 'the tuner'} to their media server by address:
      </p>
      <div className="space-y-2">
        {state.devices.map((d) => (
          <div key={d.name} className="flex items-center gap-3">
            {several && <span className="w-24 shrink-0 text-sm text-muted-foreground">{deviceTitle(d)}</span>}
            <pre className="flex-1 overflow-x-auto rounded-lg border border-border bg-muted p-4 font-mono text-sm">{`${host}:${portOf(d.listen)}`}</pre>
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        In Plex: Settings → Live TV &amp; DVR → Set Up Plex DVR → enter the network address manually.{' '}
        {state.viewers.length === 0 && 'Add a viewer first so their address is allowed.'}
      </p>
    </div>
  )
}
