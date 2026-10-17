import { AlertTriangle, WifiOff } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import type { AppState } from '@/lib/types'
import { cn } from '@/lib/utils'

function Banner({ tone, icon, children }: { tone: 'amber' | 'red'; icon: ReactNode; children: ReactNode }) {
  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-3 rounded-xl border px-4 py-3 text-sm',
        tone === 'amber' && 'border-amber-500/30 bg-amber-500/10 text-amber-200',
        tone === 'red' && 'border-red-500/30 bg-red-500/10 text-red-200',
      )}
    >
      <span className="mt-0.5 shrink-0">{icon}</span>
      <div>{children}</div>
    </div>
  )
}

/** The notices that belong at the top of every page. */
export function Banners({ state }: { state: AppState }) {
  return (
    <>
      {!state.tuner.online && (
        <Banner tone="red" icon={<WifiOff className="size-4" />}>
          <strong>The tuner is offline.</strong> Antennarr will keep looking for it
          {state.tuner.id ? ` (DeviceID ${state.tuner.id})` : ''} and follow it if it changes address.
        </Banner>
      )}
      {state.settings.mode === 'open' && (
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />}>
          <strong>Anyone who can reach this port can use your tuner.</strong> No viewers are set up yet.{' '}
          <Link to="/viewers" className="underline underline-offset-2">
            Add a viewer
          </Link>{' '}
          to restrict access to their addresses.
        </Banner>
      )}
      {!state.editable && (
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />}>
          Antennarr was started without <code>-config</code>, so changes can't be saved. Restart it with a settings
          file to add viewers or change limits.
        </Banner>
      )}
    </>
  )
}
