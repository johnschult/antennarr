import { NavLink, Outlet } from 'react-router'
import { Banners } from '@/components/banners'
import { Logo } from '@/components/logo'
import { GithubIcon } from '@/components/github-icon'
import { ThemeToggle } from '@/components/theme-toggle'
import { PlexIcon } from '@/components/plex-button'
import { useApp } from '@/hooks/app-context'
import { useAuthStatus } from '@/hooks/use-auth'
import { signOut } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import { cn } from '@/lib/utils'

const nav = [
  { to: '/', label: 'Live', end: true },
  { to: '/viewers', label: 'Viewers' },
  { to: '/setup', label: 'Setup' },
  { to: '/tuner', label: 'Tuner' },
  { to: '/settings', label: 'Settings' },
]

/** Where the project lives, for the footer. */
const REPO_URL = 'https://github.com/johnschult/antennarr'

export function Shell() {
  const { state, connected } = useApp()
  const { devices } = state
  const auth = useAuthStatus()
  const signedIn = auth.data?.signedIn ?? null
  const one = devices.length === 1 ? devices[0] : null

  return (
    <div className="flex min-h-dvh flex-col bg-[radial-gradient(ellipse_at_top,color-mix(in_oklch,var(--accent)_22%,var(--background))_0%,var(--background)_60%)]">
      <header className="sticky top-0 z-10 border-b border-border/60 bg-background/70 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:gap-8 sm:px-6">
          {/* The text lines up with the TV's body, not the antenna ears above it. */}
          <div className="flex items-end gap-2.5">
            <Logo size={42} />
            <span className="font-wordmark pb-1.5 text-2xl leading-none font-bold tracking-[0.02em]">Antennarr</span>
          </div>
          <nav aria-label="Main" className="flex gap-1 overflow-x-auto">
            {nav.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) =>
                  cn(
                    'rounded-full px-3 py-1 text-sm whitespace-nowrap',
                    isActive ? 'bg-foreground font-medium text-background' : 'text-muted-foreground hover:text-foreground',
                  )
                }
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-xs text-muted-foreground">
            <div className="hidden items-center gap-3 md:flex">
            {one ? (
              <span className="flex items-center gap-1.5" title={one.tuner.online ? 'Tuner online' : 'Tuner offline'}>
                <span className={cn('size-2 rounded-full', one.tuner.online ? 'bg-emerald-400' : 'bg-red-400')} />
                {one.label ?? one.tuner.friendlyName ?? 'Tuner'} · <span className="font-mono">{one.tuner.baseURL.replace(/^https?:\/\//, '')}</span>
              </span>
            ) : (
              <span className="flex items-center gap-1.5">
                {devices.map((d) => (
                  <span
                    key={d.name}
                    title={`${deviceTitle(d)}: ${d.tuner.online ? 'online' : 'offline'}`}
                    className={cn('size-2 rounded-full', d.tuner.online ? 'bg-emerald-400' : 'bg-red-400')}
                  />
                ))}
                {devices.length} tuners
              </span>
            )}
            <span
              data-testid="connection"
              className={cn('rounded-full px-2 py-0.5', connected ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-amber-500/10 text-amber-700 dark:text-amber-300')}
            >
              {connected ? 'Live' : 'Reconnecting…'}
            </span>
            </div>
            {signedIn && (
              <button
                type="button"
                onClick={() => void signOut().finally(() => window.location.reload())}
                title="Sign out"
                className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1 text-xs hover:bg-muted"
              >
                <PlexIcon className="size-4" />
                <span className="max-w-24 truncate">{signedIn.name}</span>
                <span className="text-muted-foreground">· Sign out</span>
              </button>
            )}
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-7xl flex-1 space-y-6 px-4 py-8 sm:px-6">
        <Banners state={state} />
        <Outlet />
      </main>
      <footer className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-2 px-4 pb-8 text-xs text-muted-foreground sm:px-6">
        <span>
          Antennarr{' '}
          <a href={state.versionUrl} target="_blank" rel="noopener noreferrer" data-testid="version" className="font-mono underline-offset-2 hover:text-foreground hover:underline">
            {state.version}
          </a>
        </span>
        <a
          href={REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          aria-label="Antennarr on GitHub"
          title="Antennarr on GitHub"
          className="inline-flex size-8 items-center justify-center rounded-md hover:bg-muted hover:text-foreground"
        >
          <GithubIcon className="size-5" />
        </a>
      </footer>
    </div>
  )
}
