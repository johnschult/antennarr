import { cn } from '@/lib/utils'

/** A small set of options where exactly one is chosen, shown as buttons. */
export function Choice<T extends string>({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: T
  onChange: (next: T) => void
  options: { id: T; label: string; hint?: string }[]
}) {
  return (
    <div role="radiogroup" aria-label={label} className="grid gap-2 sm:grid-cols-[repeat(auto-fit,minmax(9rem,1fr))]">
      {options.map((o) => {
        const on = o.id === value
        return (
          <button
            key={o.id}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(o.id)}
            className={cn(
              'rounded-lg border px-3 py-2 text-left text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
              on ? 'border-primary bg-primary/10' : 'border-border bg-card/70 hover:bg-muted',
            )}
          >
            <span className="block font-medium">{o.label}</span>
            {o.hint && <span className="mt-0.5 block text-xs text-muted-foreground">{o.hint}</span>}
          </button>
        )
      })}
    </div>
  )
}
