import { AlertTriangle, WifiOff, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { tunerPath } from '@/lib/tuner-tabs'
import { deviceTitle } from '@/lib/device'
import type { AppState } from '@/lib/types'
import { cn } from '@/lib/utils'

const DOCS = 'https://github.com/johnschult/yagi/blob/main/docs'

/** Where this browser remembers the notices that were dismissed. */
const DISMISSED_KEY = 'yagi.dismissedNotices'

function dismissed(key: string): boolean {
  try {
    return (localStorage.getItem(DISMISSED_KEY) ?? '').split(',').includes(key)
  } catch {
    return false
  }
}
function remember(key: string) {
  try {
    const kept = (localStorage.getItem(DISMISSED_KEY) ?? '').split(',').filter(Boolean)
    localStorage.setItem(DISMISSED_KEY, [...kept, key].join(','))
  } catch {
    // nowhere to remember it: it shows again next time, which is harmless
  }
}

/**
 * One notice. `help` is a link to the part of the docs that explains it. With a `dismissKey` it can be closed,
 * and this browser then stays quiet about it; the others go away by themselves once the cause does.
 */
export function Banner({ tone, icon, children, help, dismissKey }: { tone: 'amber' | 'red' | 'blue'; icon: ReactNode; children: ReactNode; help?: string; dismissKey?: string }) {
  const [closed, setClosed] = useState(() => dismissKey !== undefined && dismissed(dismissKey))
  if (closed) return null
  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-3 rounded-xl border px-4 py-3 text-sm',
        tone === 'amber' && 'border-amber-500/30 bg-amber-500/10 text-amber-900 dark:text-amber-200',
        tone === 'red' && 'border-red-500/30 bg-red-500/10 text-red-900 dark:text-red-200',
        tone === 'blue' && 'border-sky-500/30 bg-sky-500/10 text-sky-900 dark:text-sky-200',
      )}
    >
      <span className="mt-0.5 shrink-0">{icon}</span>
      <div className="grow">
        {children}
        {help && (
          <>
            {' '}
            <a href={`${DOCS}/${help}`} target="_blank" rel="noreferrer" className="whitespace-nowrap underline underline-offset-2">
              Troubleshooting
            </a>
          </>
        )}
      </div>
      {dismissKey && (
        <button
          type="button"
          aria-label="Dismiss"
          className="-m-1 shrink-0 rounded p-1 opacity-70 hover:opacity-100"
          onClick={() => {
            remember(dismissKey)
            setClosed(true)
          }}
        >
          <X className="size-4" />
        </button>
      )}
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
        <Banner tone="red" icon={<WifiOff className="size-4" />} help="troubleshooting.md#the-tuner-shows-as-offline">
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
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />} help="getting-started.md#4-add-a-viewer">
          <strong>Nobody can use your {several ? 'tuners' : 'tuner'} yet.</strong> No viewers are set up.{' '}
          <Link to="/viewers?add" className="underline underline-offset-2">
            Add a viewer
          </Link>{' '}
          to let a media server in.
        </Banner>
      )}
      {state.editable && state.settings.admin.length > 0 && !state.settings.plex.enabled && (
        <Banner tone="amber" icon={<AlertTriangle className="size-4" />} dismissKey="admin-address" help="troubleshooting.md#this-address-cant-open-the-admin-page">
          <strong>Your way into this page is this computer's address.</strong> If your router gives it a new one, you
          will be locked out. Reserve its address in your router (a DHCP reservation) so it stays the same, or, if you
          use Plex,{' '}
          <Link to="/settings" className="underline underline-offset-2">
            turn on Sign in with Plex
          </Link>{' '}
          to always have a way back in.
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
