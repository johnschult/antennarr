import { History, Radio, RadioReceiver, SlidersHorizontal, Users, type LucideIcon } from 'lucide-react'
import { Link, NavLink, Outlet } from 'react-router'
import { Banners, ConnectionBanner } from '@/components/banners'
import { WhatsNewDialog } from '@/components/whats-new-dialog'
import { Logo } from '@/components/logo'
import { GithubIcon } from '@/components/github-icon'
import { ThemeToggle } from '@/components/theme-toggle'
import { PlexIcon } from '@/components/plex-button'
import { useApp } from '@/hooks/app-context'
import { useAuthStatus } from '@/hooks/use-auth'
import { useIsPhone } from '@/hooks/use-media-query'
import { useRestartToasts } from '@/hooks/use-restart-toasts'
import { useUpgradeToasts } from '@/hooks/use-upgrade-toasts'
import { useWhatsNew } from '@/hooks/use-whats-new'
import { signOut } from '@/lib/api'
import { cn } from '@/lib/utils'

// The pages, in the order they are used: watching, who may watch, the hardware,
// then everything else.
const nav: { to: string; label: string; icon: LucideIcon; end?: boolean; needs?: 'history' }[] = [
  { to: '/', label: 'Live', icon: Radio, end: true },
  { to: '/viewers', label: 'Viewers', icon: Users },
  { to: '/tuners', label: 'Tuners', icon: RadioReceiver },
  { to: '/activity', label: 'Activity', icon: History, needs: 'history' },
  { to: '/settings', label: 'Settings', icon: SlidersHorizontal },
]

/** A page's name in the top bar, with its icon on the current page only. */
function NavLabel({ item, active }: { item: (typeof nav)[number]; active: boolean }) {
  return (
    <>
      {active && <item.icon aria-hidden className="size-4 shrink-0" />}
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

/**
 * On a phone the pages are a bar along the bottom, like an iPhone app's tabs: always in reach of a thumb, every page
 * with its icon and name, the current one marked. It leaves room for the home indicator.
 */
function BottomBar({ pages }: { pages: typeof nav }) {
  return (
    <nav aria-label="Main" className="fixed inset-x-0 bottom-0 z-20 border-t border-border/60 bg-background/90 pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] backdrop-blur">
      <ul className="mx-auto flex max-w-xl">
        {pages.map((n) => (
          <li key={n.to} className="min-w-0 flex-1">
            <NavLink
              to={n.to}
              end={n.end}
              className={({ isActive }) =>
                cn(
                  'flex h-14 flex-col items-center justify-center gap-1 text-[11px] outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
                  isActive ? 'font-medium text-foreground' : 'text-muted-foreground',
                )
              }
            >
              {({ isActive }) => (
                <>
                  <span className={cn('grid h-7 w-12 place-items-center rounded-full transition-colors', isActive && 'bg-foreground/10')}>
                    <n.icon aria-hidden className="size-5" strokeWidth={isActive ? 2.5 : 2} />
                  </span>
                  <span className="max-w-full truncate px-1">{n.label}</span>
                </>
              )}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  )
}

/** The version as a person writes it: v2026.10.10. A development build's own label is left as it is. */
const versionLabel = (v: string) => (/^\d/.test(v) ? `v${v}` : v)

export function Shell() {
  const { state } = useApp()
  const whatsNew = useWhatsNew(state.version)
  // A page that depends on a setting that is off isn't offered.
  const pages = nav.filter((n) => n.needs !== 'history' || state.settings.history)
  useUpgradeToasts()
  useRestartToasts()
  const phone = useIsPhone()

  return (
    <div className={cn('flex min-h-dvh flex-col bg-background pr-[env(safe-area-inset-right)] pl-[env(safe-area-inset-left)]', phone && 'pb-[calc(3.5rem+env(safe-area-inset-bottom))]')}>
      <header className="sticky top-0 z-10 border-b border-border/60 bg-background/70 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:px-6 md:gap-8">
          {/* The text lines up with the TV's body, not the antenna ears above it. */}
          <Link to="/" title="Live" className="flex items-end gap-2.5 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <Logo size={42} />
            <span className="font-wordmark pb-1 text-4xl leading-none font-bold tracking-[0.02em]">Yagi</span>
          </Link>
          {!phone && (
            <nav aria-label="Main" className="flex gap-1">
              {pages.map((n) => (
                <NavLink key={n.to} to={n.to} end={n.end} className={linkClass}>
                  {({ isActive }) => <NavLabel item={n} active={isActive} />}
                </NavLink>
              ))}
            </nav>
          )}
          <div className="ml-auto flex items-center gap-2 text-xs md:gap-3 text-muted-foreground">
            <ConnectionStatus />
            {!phone && <SignedIn />}
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-7xl flex-1 space-y-6 px-4 py-8 sm:px-6">
        <ConnectionBanner />
        <Banners state={state} />
        <Outlet />
      </main>
      <WhatsNewDialog open={whatsNew.open} onClose={whatsNew.dismiss} version={whatsNew.current} releases={whatsNew.releases} updated={whatsNew.byItself} />
      <footer className="mx-auto flex w-full max-w-7xl flex-col items-center justify-center gap-1 px-4 pb-8 text-xs text-muted-foreground sm:px-6">
        <p>
          Point your antenna <strong className="font-semibold text-foreground">anywhere</strong>.
        </p>
        {phone && (
          <div className="flex justify-center py-1">
            <SignedIn />
          </div>
        )}
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
            {versionLabel(state.version)}
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
      {phone && <BottomBar pages={pages} />}
    </div>
  )
}
