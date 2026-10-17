import type { ReactNode } from 'react'
import { Label } from '@/components/ui/label'

/** A group of related settings: a heading, then one bordered list of rows. */
export function Section({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="space-y-2">
      <div className="px-1">
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
      </div>
      <div className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card/70">{children}</div>
    </section>
  )
}

/** One setting: what it is on the left, how to change it on the right. */
export function Row({
  title,
  description,
  htmlFor,
  error,
  children,
}: {
  title: string
  description?: ReactNode
  /** The id of the control, so the title labels it. */
  htmlFor?: string
  error?: string | null
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center sm:justify-between sm:gap-8">
      <div className="min-w-0 sm:max-w-md">
        <Label htmlFor={htmlFor} className="font-medium">
          {title}
        </Label>
        {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        {error && (
          <p role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">{children}</div>
    </div>
  )
}
