import { AlertTriangle, WifiOff } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { tunerPath } from '@/lib/tuner-tabs'
import { deviceTitle } from '@/lib/device'
import type { AppState } from '@/lib/types'
import { cn } from '@/lib/utils'

function Banner({ tone, icon, children }: { tone: 'amber' | 'red'; icon: ReactNode; children: ReactNode }) {
  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-3 rounded-xl border px-4 py-3 text-sm',
        tone === 'amber' && 'border-amber-500/30 bg-amber-500/10 text-amber-900 dark:text-amber-200',
        tone === 'red' && 'border-red-500/30 bg-red-500/10 text-red-900 dark:text-red-200',
      )}
    >
      <span className="mt-0.5 shrink-0">{icon}</span>
      <div>{children}</div>
    </div>
  )
}

/** The notices that belong at the top of every page. */
export function Banners({ state }: { state: AppState }) {
  // A tuner that is restarting to install firmware is expected to be away: that is not an outage.
  const offline = state.devices.filter((d) => !d.tuner.online && !d.tuner.updating && !d.tuner.restarting)
  const several = state.devices.length > 1

  return (
    <>
      {offline.length > 0 && (
        <Banner tone="red" icon={<WifiOff className="size-4" />}>
          {several ? (
            <>
              <strong>{offline.length === 1 ? 'A tuner is offline' : `${offline.length} tuners are offline`}:</strong>{' '}
              {offline.map((d, i) => (
                <span key={d.name}>
                  {i > 0 && ', '}
                  <Link to={tunerPath(d.name)} className="underline underline-offset-2">
                    {deviceTitle(d)}
                  </Link>
                </span>
              ))}
              .
            </>
          ) : (
            <>
              <strong>
                <Link to={tunerPath(offline[0].name)} className="underline underline-offset-2">
                  The tuner is offline.
                </Link>
              </strong>
              {offline[0].tuner.id ? ` (DeviceID ${offline[0].tuner.id})` : ''}
            </>
          )}{' '}
          Yagi will keep looking and follow {several ? 'them' : 'it'} if the address changes.
        </Banner>
      )}
      {state.settings.mode === 'no-viewers' && (
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />}>
          <strong>Nobody can use your {several ? 'tuners' : 'tuner'} yet.</strong> No viewers are set up.{' '}
          <Link to="/viewers?add" className="underline underline-offset-2">
            Add a viewer
          </Link>{' '}
          to let a media server in.
        </Banner>
      )}
      {!state.editable && (
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />}>
          Yagi was started without <code>-config</code>, so changes can't be saved. Restart it with a settings
          file to add viewers or change limits.
        </Banner>
      )}
    </>
  )
}
