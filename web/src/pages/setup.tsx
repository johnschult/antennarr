import { useApp } from '@/hooks/app-context'

/** Placeholder: the generated setup commands are the next milestone. */
export function SetupPage() {
  const { state } = useApp()
  const here = window.location.host
  return (
    <div className="max-w-2xl space-y-4">
      <h1 className="text-xl font-semibold tracking-tight">Setup guide</h1>
      <p className="text-sm text-zinc-400">
        Copy-paste commands for each viewer's firewall, routing and media server are coming next. For now, a viewer adds
        the tuner to their media server by address:
      </p>
      <pre className="overflow-x-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 font-mono text-sm">{here}</pre>
      <p className="text-xs text-zinc-500">
        In Plex: Settings → Live TV &amp; DVR → Set Up Plex DVR → enter the network address manually.{' '}
        {state.viewers.length === 0 && 'Add a viewer first so their address is allowed.'}
      </p>
    </div>
  )
}
