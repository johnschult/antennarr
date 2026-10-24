import { AlertTriangle, Ban, ListX, WifiOff, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { useApp } from '@/hooks/app-context'
import { tunerPath } from '@/lib/tuner-tabs'
import { deviceTitle } from '@/lib/device'
import type { AppState, ProblemKind } from '@/lib/types'
import { timeAgo } from '@/lib/format'
import { recentProblems } from '@/lib/problems'
import { cn } from '@/lib/utils'

const DOCS = 'https://yagi.johnschult.com'

/** The address of a docs page, from "troubleshooting.md#the-tuner-shows-as-offline". */
function docsUrl(help: string): string {
  const [page, anchor] = help.replace(/\.md/, '').split('#')
  return `${DOCS}/${page}/${anchor ? `#${anchor}` : ''}`
}

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
    localStorage.setItem(DISMISSED_KEY, [...kept, key].slice(-50).join(',')) // the newest fifty: problems come in episodes, so this can grow
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
            <a href={docsUrl(help)} target="_blank" rel="noreferrer" className="whitespace-nowrap underline underline-offset-2">
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

/** "http://192.168.1.96" as "192.168.1.96". */
const hostOf = (base: string) => base.replace(/^https?:\/\//, '').replace(/\/$/, '')

const problemIcon: Record<ProblemKind, ReactNode> = {
  refused: <Ban className="size-4" />,
  denied: <Ban className="size-4" />,
  'no-channels': <ListX className="size-4" />,
}

/** What the owner needs to know about it, since the media server only says "channel not available" or shows an empty guide. */
const problemNote: Record<ProblemKind, string> = {
  refused: 'Media servers show "channel not available" when this happens.',
  denied: 'The media server shows "channel not available" when this happens.',
  'no-channels': 'The guide in a media server looks empty until it has its channels again. That can happen during a rescan, or just after the tuner restarts.',
}

const problemHelp: Record<ProblemKind, string> = {
  refused: 'troubleshooting.md#a-media-server-says-the-channel-isnt-available-or-the-guide-is-empty',
  denied: 'troubleshooting.md#a-503-when-watching',
  'no-channels': 'troubleshooting.md#a-media-server-says-the-channel-isnt-available-or-the-guide-is-empty',
}

/** The notices that belong at the top of every page. */
export function Banners({ state }: { state: AppState }) {
  // A tuner that is restarting to install firmware is expected to be away: that is not an outage.
  const offline = state.devices.filter((d) => !d.tuner.online && !d.tuner.updating && !d.tuner.restarting)
  const several = state.devices.length > 1
  const problems = recentProblems(state)

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
                  The tuner isn't answering{offline[0].tuner.baseURL ? ` at ${hostOf(offline[0].tuner.baseURL)}` : ''}.
                </Link>
              </strong>
            </>
          )}{' '}
          {offline.every((d) => d.tuner.search && !d.tuner.search.found)
            ? `Yagi searched the network for ${several ? 'them' : 'it'} and found nothing. Is ${several ? 'each' : 'it'} powered on and on the same network? `
            : ''}
          Yagi will keep looking and follow {several ? 'them' : 'it'} if the address changes.
        </Banner>
      )}
      {problems.map(({ device, p }) => (
        <Banner key={`${device.name}-${p.kind}-${p.time}`} tone={p.kind === 'no-channels' ? 'amber' : 'blue'} icon={problemIcon[p.kind]} help={problemHelp[p.kind]} dismissKey={`${device.name}-${p.kind}-${p.time}`}>
          <strong>
            {p.text}
            {several && p.kind !== 'denied' ? ` (${deviceTitle(device)})` : ''}.
          </strong>{' '}
          {problemNote[p.kind]} <span className="opacity-80">({timeAgo(p.last, new Date(state.time))})</span>
        </Banner>
      ))}
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

/** How long the page can be without Yagi before it says so. A reconnect that takes a moment isn't worth a banner. */
const LOST_FOR = 3000

/**
 * Said at the top of every page when the page has lost touch with Yagi itself (not the tuner): what is shown may be
 * out of date, nothing can be changed, and the page is trying again by itself.
 */
export function ConnectionBanner() {
  const { connected } = useApp()
  const [lost, setLost] = useState(false)
  useEffect(() => {
    if (connected) {
      setLost(false)
      return
    }
    const t = setTimeout(() => setLost(true), LOST_FOR)
    return () => clearTimeout(t)
  }, [connected])
  if (!lost) return null
  return (
    <Banner tone="red" icon={<WifiOff className="size-4" />} help="troubleshooting.md#im-signed-out-after-a-restart-or-the-page-says-reconnecting">
      <strong>This page can't reach Yagi.</strong> What it shows may be out of date, and changes can't be saved until Yagi is back. That happens while Yagi restarts or updates, or when this computer or phone has lost the network or Yagi's address. The page keeps trying by itself and catches up when it can.
    </Banner>
  )
}
