import type { LucideIcon } from 'lucide-react'
import { Link } from 'react-router'
import { cn } from '@/lib/utils'

/**
 * Tabs, as links, so each has an address that can be opened, shared and gone back to. On a phone they share the
 * width, each with its icon over its name, so nothing scrolls.
 */
export function TabLinks<T extends string>({ label, tabs, current, to }: { label: string; tabs: { id: T; label: string; icon: LucideIcon }[]; current: T; to: (id: T) => string }) {
  return (
    <nav aria-label={label} className="border-b border-border">
      <ul className="flex gap-1">
        {tabs.map((t) => {
          const active = t.id === current
          return (
            <li key={t.id} className="min-w-0 flex-1 sm:flex-none">
              <Link
                to={to(t.id)}
                aria-current={active ? 'page' : undefined}
                className={cn(
                  '-mb-px flex flex-col items-center gap-0.5 border-b-2 px-1 py-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring sm:inline-flex sm:flex-row sm:gap-1.5 sm:px-3 sm:text-sm',
                  active ? 'border-primary font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
                )}
              >
                <t.icon aria-hidden className="size-4" />
                {t.label}
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
