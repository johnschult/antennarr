import { NavLink, Outlet } from 'react-router'
import { Banners } from '@/components/banners'
import { Logo } from '@/components/logo'
import { useApp } from '@/hooks/app-context'
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
  const { tuner } = state

  return (
    <div className="min-h-screen bg-[radial-gradient(ellipse_at_top,#0e2a33_0%,#09090b_55%)]">
      <header className="sticky top-0 z-10 border-b border-zinc-800/60 bg-zinc-950/60 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:gap-8 sm:px-6">
          <div className="flex items-center gap-2">
            <Logo size={24} />
            <span className="font-semibold">Antennarr</span>
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
                    isActive ? 'bg-zinc-100 font-medium text-zinc-900' : 'text-zinc-400 hover:text-zinc-100',
                  )
                }
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto hidden items-center gap-3 text-xs text-zinc-400 md:flex">
            <span className="flex items-center gap-1.5" title={tuner.online ? 'Tuner online' : 'Tuner offline'}>
              <span className={cn('size-2 rounded-full', tuner.online ? 'bg-emerald-400' : 'bg-red-400')} />
              {tuner.friendlyName ?? 'Tuner'} · <span className="font-mono">{tuner.baseURL.replace(/^https?:\/\//, '')}</span>
            </span>
            <span
              data-testid="connection"
              className={cn('rounded-full px-2 py-0.5', connected ? 'bg-emerald-500/10 text-emerald-300' : 'bg-amber-500/10 text-amber-300')}
            >
              {connected ? 'Live' : 'Reconnecting…'}
            </span>
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
