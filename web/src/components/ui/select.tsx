import { ChevronDown } from 'lucide-react'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

/** A native select (so a phone shows its own picker), with a caret of our own that sits inside the box with room around it. */
function Select({ className, children, ...props }: ComponentProps<'select'>) {
  return (
    <div className="relative">
      <select
        data-slot="select"
        className={cn(
          'h-9 w-full appearance-none rounded-lg border border-input bg-background pr-9 pl-3 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown aria-hidden className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  )
}

export { Select }
