import { NavLink, Outlet } from 'react-router'
import { Banners } from '@/components/banners'
import { Logo } from '@/components/logo'
import { ThemeToggle } from '@/components/theme-toggle'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'
import { cn } from '@/lib/utils'

const nav = [
  { to: '/', label: 'Live', end: true },
  { to: '/viewers', label: 'Viewers' },
  { to: '/setup', label: 'Setup' },
  { to: '/tuner', label: 'Tuner' },
  { to: '/settings', label: 'Settings' },
]

export function Shell() {
  const { state, connected } = useApp()
  const { devices } = state
  const one = devices.length === 1 ? devices[0] : null

  return (
    <div className="min-h-screen bg-[radial-gradient(ellipse_at_top,color-mix(in_oklch,var(--accent)_22%,var(--background))_0%,var(--background)_60%)]">
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
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl space-y-6 px-4 py-8 sm:px-6">
        <Banners state={state} />
        <Outlet />
      </main>
    </div>
  )
}
