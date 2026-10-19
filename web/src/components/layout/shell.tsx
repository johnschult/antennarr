import { History, RadioReceiver, Menu, Radio, SlidersHorizontal, Users, X, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router'
import { Banners } from '@/components/banners'
import { WhatsNewDialog } from '@/components/whats-new-dialog'
import { Logo } from '@/components/logo'
import { GithubIcon } from '@/components/github-icon'
import { ThemeToggle } from '@/components/theme-toggle'
import { PlexIcon } from '@/components/plex-button'
import { useApp } from '@/hooks/app-context'
import { useAuthStatus } from '@/hooks/use-auth'
import { useRestartToasts } from '@/hooks/use-restart-toasts'
import { useUpgradeToasts } from '@/hooks/use-upgrade-toasts'
import { useWhatsNew } from '@/hooks/use-whats-new'
import { signOut } from '@/lib/api'
import { cn } from '@/lib/utils'

// The pages, in the order they are used: watching, who may watch, the hardware,
// then everything else. The path of /tuner is older than its name here and stays,
// so existing links keep working.
const nav: { to: string; label: string; icon: LucideIcon; end?: boolean; needs?: 'history' }[] = [
  { to: '/', label: 'Live', icon: Radio, end: true },
  { to: '/viewers', label: 'Viewers', icon: Users },
  { to: '/tuner', label: 'Tuners', icon: RadioReceiver },
  { to: '/activity', label: 'Activity', icon: History, needs: 'history' },
  { to: '/settings', label: 'Settings', icon: SlidersHorizontal },
]

/** A page's name with its icon: always in the phone menu, which has room, and only on the current page in the top bar. */
function NavLabel({ item, active, always }: { item: (typeof nav)[number]; active: boolean; always?: boolean }) {
  return (
    <>
      {(active || always) && <item.icon aria-hidden className="size-4 shrink-0" />}
      {item.label}
    </>
  )
}

/** Where the project lives, for the footer. */
const REPO_URL = 'https://github.com/johnschult/yagi'

/**
 * Whether the page's live feed is connected. Nothing here about the tuners: there can be
 * any number, so each has its header on the Live page and a row on the Tuners page, and
 * an offline one is a banner.
 */
function ConnectionStatus() {
  const { connected } = useApp()
  return (
    <span
      data-testid="connection"
      className={cn('rounded-full px-2 py-0.5', connected ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-amber-500/10 text-amber-700 dark:text-amber-300')}
    >
      <span className="inline-flex items-center gap-1.5">
        <span aria-hidden className="relative flex size-1.5">
          {connected && <span className="absolute inline-flex size-full rounded-full bg-emerald-500 opacity-60 motion-safe:animate-ping" />}
          <span className={cn('relative inline-flex size-1.5 rounded-full', connected ? 'bg-emerald-500' : 'bg-amber-500')} />
        </span>
        {connected ? 'Live' : 'Reconnecting…'}
      </span>
    </span>
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
  cn('inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm whitespace-nowrap', isActive ? 'bg-foreground font-medium text-background' : 'text-muted-foreground hover:text-foreground')

const menuLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    'flex items-center gap-3 rounded-lg px-3 py-3 text-base',
    isActive ? 'bg-foreground font-medium text-background' : 'text-foreground hover:bg-muted',
  )

export function Shell() {
  const { state } = useApp()
  const whatsNew = useWhatsNew(state.version)
  // A page that depends on a setting that is off isn't offered.
  const pages = nav.filter((n) => n.needs !== 'history' || state.settings.history)
  useUpgradeToasts()
  useRestartToasts()
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
    <div className="flex min-h-dvh flex-col bg-background">
      <header className="sticky top-0 z-10 border-b border-border/60 bg-background/70 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:px-6 md:gap-8">
          {/* The text lines up with the TV's body, not the antenna ears above it. */}
          <Link to="/" title="Live" className="flex items-end gap-2.5 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <Logo size={42} />
            <span className="font-wordmark pb-1 text-4xl leading-none font-bold tracking-[0.02em]">Yagi</span>
          </Link>
          <nav aria-label="Main" className="hidden gap-1 md:flex">
            {pages.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={linkClass}>
                {({ isActive }) => <NavLabel item={n} active={isActive} />}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 text-xs text-muted-foreground">
            <div className="hidden items-center gap-3 md:flex">
              <ConnectionStatus />
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
                {pages.map((n) => (
                  <NavLink key={n.to} to={n.to} end={n.end} className={menuLinkClass}>
                    {({ isActive }) => <NavLabel item={n} active={isActive} always />}
                  </NavLink>
                ))}
              </nav>
              <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-3 gap-y-2 border-t border-border/60 px-4 py-3 text-xs text-muted-foreground sm:px-6">
                <ConnectionStatus />
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
      <WhatsNewDialog open={whatsNew.open} onClose={whatsNew.dismiss} version={whatsNew.current} releases={whatsNew.releases} updated={whatsNew.byItself} />
      <footer className="mx-auto flex w-full max-w-7xl flex-col items-center justify-center gap-1 px-4 pb-8 text-xs text-muted-foreground sm:px-6">
        <p>
          Point your antenna <strong className="font-semibold text-foreground">anywhere</strong>.
        </p>
        <div className="flex items-center gap-1">
          {/* The version is here, not under the wordmark, which is big enough now to cover it. */}
          <a
            href={state.versionUrl}
            target="_blank"
            rel="noopener noreferrer"
            data-testid="version"
            title="Release notes for this version"
            className="rounded-md px-2 py-1 font-mono underline-offset-2 hover:bg-muted hover:text-foreground"
          >
            {state.version}
          </a>
          {whatsNew.canShow && (
            <button type="button" onClick={whatsNew.show} className="rounded-md px-2 py-1 underline-offset-2 hover:bg-muted hover:text-foreground">
              What's new
            </button>
          )}
          <Link to="/settings/logs" className="rounded-md px-2 py-1 underline-offset-2 hover:bg-muted hover:text-foreground">
            Logs
          </Link>
          <a href={state.docsUrl} target="_blank" rel="noopener noreferrer" className="rounded-md px-2 py-1 underline-offset-2 hover:bg-muted hover:text-foreground">
            Help
          </a>
          <a
            href={REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Yagi on GitHub"
            title="Yagi on GitHub"
            className="inline-flex size-8 items-center justify-center rounded-md hover:bg-muted hover:text-foreground"
          >
            <GithubIcon className="size-5" />
          </a>
        </div>
      </footer>
    </div>
  )
}
