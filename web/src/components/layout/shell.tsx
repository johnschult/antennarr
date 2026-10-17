import { Menu, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router'
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

/** Whether the tuner is up and whether the live feed is connected. */
function Status() {
  const { state, connected } = useApp()
  const { devices } = state
  const one = devices.length === 1 ? devices[0] : null
  return (
    <>
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
    </>
  )
}

/** Who is signed in with Plex, with a way to sign out. Nothing when no one is. */
function SignedIn() {
  const auth = useAuthStatus()
  const signedIn = auth.data?.signedIn ?? null
  if (!signedIn) return null
  return (
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
  )
}

const linkClass = ({ isActive }: { isActive: boolean }) =>
  cn('rounded-full px-3 py-1 text-sm whitespace-nowrap', isActive ? 'bg-foreground font-medium text-background' : 'text-muted-foreground hover:text-foreground')

const menuLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'flex items-center rounded-lg px-3 py-3 text-base',
    isActive ? 'bg-foreground font-medium text-background' : 'text-foreground hover:bg-muted',
  )

export function Shell() {
  const { state } = useApp()
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const button = useRef<HTMLButtonElement>(null)

  // Going to a page, or pressing Escape, puts the menu away.
  useEffect(() => setOpen(false), [location.pathname])
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        button.current?.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <div className="flex min-h-dvh flex-col bg-[radial-gradient(ellipse_at_top,color-mix(in_oklch,var(--accent)_22%,var(--background))_0%,var(--background)_60%)]">
      <header className="sticky top-0 z-10 border-b border-border/60 bg-background/70 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:px-6 md:gap-8">
          {/* The text lines up with the TV's body, not the antenna ears above it. */}
          <div className="flex items-end gap-2.5">
            <Logo size={42} />
            <span className="font-wordmark pb-1.5 text-2xl leading-none font-bold tracking-[0.02em]">Antennarr</span>
          </div>
          <nav aria-label="Main" className="hidden gap-1 md:flex">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={linkClass}>
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-xs text-muted-foreground">
            <div className="hidden items-center gap-3 md:flex">
              <Status />
            </div>
            <div className="hidden md:block">
              <SignedIn />
            </div>
            <ThemeToggle />
            <button
              ref={button}
              type="button"
              onClick={() => setOpen((o) => !o)}
              aria-expanded={open}
              aria-controls="mobile-menu"
              aria-label={open ? 'Close menu' : 'Open menu'}
              className="grid size-10 place-items-center rounded-lg border border-border bg-card text-foreground hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:hidden"
            >
              {open ? <X className="size-5" /> : <Menu className="size-5" />}
            </button>
          </div>
        </div>
        {/* On a phone the pages, and what the header has no room for, live here. Only built while open. */}
        <div id="mobile-menu" className="md:hidden">
          {open && (
            <div className="border-t border-border/60 bg-background/95">
              <nav aria-label="Main menu" className="mx-auto flex max-w-7xl flex-col gap-1 px-4 py-3 sm:px-6">
                {nav.map((n) => (
                  <NavLink key={n.to} to={n.to} end={n.end} className={menuLinkClass}>
                    {n.label}
                  </NavLink>
                ))}
              </nav>
              <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-3 gap-y-2 border-t border-border/60 px-4 py-3 text-xs text-muted-foreground sm:px-6">
                <Status />
                <SignedIn />
              </div>
            </div>
          )}
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
        <div className="flex items-center gap-1">
          <a href={state.docsUrl} target="_blank" rel="noopener noreferrer" className="rounded-md px-2 py-1 underline-offset-2 hover:bg-muted hover:text-foreground">
            Help
          </a>
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
        </div>
      </footer>
    </div>
  )
}
