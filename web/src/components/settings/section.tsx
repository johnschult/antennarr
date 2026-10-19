import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Label } from '@/components/ui/label'

/** A group of related settings: a heading, then one bordered list of rows. */
export function Section({ title, description, icon: Icon, bare = false, children }: { title: string; description?: string; icon?: LucideIcon; /** On a page that already says what it is, such as a tab: no heading, only the description. */ bare?: boolean; children: ReactNode }) {
  return (
    <section aria-label={title} className="space-y-2">
      {(!bare || description) && (
        <div className="px-1">
          {!bare && (
            <h2 className="flex items-center gap-2 text-sm font-semibold">
              {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
              {title}
            </h2>
          )}
          {description && <p className={`text-xs text-muted-foreground${bare ? '' : ` mt-0.5${Icon ? ' pl-6' : ''}`}`}>{description}</p>}
        </div>
      )}
      <div className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">{children}</div>
    </section>
  )
}

/** One setting: what it is on the left, how to change it on the right. */
export function Row({
  title,
  description,
  icon,
  htmlFor,
  error,
  children,
}: {
  title: string
  description?: ReactNode
  /** A decorative mark before the title, to set the row off. */
  icon?: ReactNode
  /** The id of the control, so the title labels it. */
  htmlFor?: string
  error?: string | null
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center sm:justify-between sm:gap-8">
      <div className="flex min-w-0 items-start gap-3 sm:max-w-md">
        {icon && <span className="mt-0.5 shrink-0">{icon}</span>}
        <div className="min-w-0">
          <Label htmlFor={htmlFor} className="font-medium">
            {title}
          </Label>
          {description && <p className="mt-1.5 text-xs text-muted-foreground">{description}</p>}
          {error && (
            <p role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">
              {error}
            </p>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">{children}</div>
    </div>
  )
}
