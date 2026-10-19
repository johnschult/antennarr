import { Check, ListFilter, X } from 'lucide-react'
import { type ReactNode, useMemo } from 'react'
import { LanguageFlag } from '@/components/language-flag'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTitle, PopoverTrigger } from '@/components/ui/popover'
import { categoryLooks, languageName } from '@/lib/channel-about'
import { type Facets, facetCount, facetOptions, NO_FACETS } from '@/lib/channel-filters'
import type { TunerChannel } from '@/lib/types'
import { cn } from '@/lib/utils'

/** A choice you can switch on and off, with how many channels it is on. */
function Chip({ on, onClick, count, children }: { on: boolean; onClick: () => void; count?: number; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn(
        'inline-flex min-h-8 items-center gap-1.5 rounded-full border py-1 pr-2.5 pl-2.5 text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
        on ? 'border-primary bg-primary/15 text-foreground' : 'border-border bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground',
      )}
    >
      {on && <Check aria-hidden className="size-3.5 text-primary" />}
      {children}
      {count !== undefined && <span className="tabular-nums opacity-70">{count}</span>}
    </button>
  )
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="space-y-2">
      <h3 className="text-[11px] tracking-wide text-muted-foreground uppercase">{title}</h3>
      <div className="flex flex-wrap gap-1.5">{children}</div>
    </section>
  )
}

const toggled = <T,>(set: ReadonlySet<T>, v: T): Set<T> => {
  const next = new Set(set)
  if (!next.delete(v)) next.add(v)
  return next
}

/** The Filters button, with how many are on, and the panel it opens: quality, category and language. */
export function ChannelFilters({ channels, facets, onChange, shown }: { channels: TunerChannel[]; facets: Facets; onChange: (f: Facets) => void; shown: number }) {
  const opts = useMemo(() => facetOptions(channels), [channels])
  const n = facetCount(facets)
  const looks = useMemo(() => new Map(channels.flatMap((c) => categoryLooks(c.about?.categories)).map((l) => [l.label, l])), [channels])
  return (
    <Popover>
      <PopoverTrigger render={<Button variant={n > 0 ? 'default' : 'outline'} size="sm" />} aria-label={n > 0 ? `Filters, ${n} on` : 'Filters'}>
        <ListFilter data-icon="inline-start" /> Filters
        {n > 0 && <span className="ml-1 rounded-full bg-background/25 px-1.5 text-[11px] tabular-nums">{n}</span>}
      </PopoverTrigger>
      <PopoverContent className="max-h-[min(34rem,75dvh)] w-[22rem] overflow-y-auto" align="end">
        <div data-testid="filters" className="space-y-5 p-4">
          <div className="flex items-center justify-between">
            <PopoverTitle>Filters</PopoverTitle>
            <Button variant="ghost" size="sm" disabled={n === 0} onClick={() => onChange(NO_FACETS)}>
              Clear
            </Button>
          </div>

          <Group title="Quality">
            <Chip on={facets.hd} count={opts.hd} onClick={() => onChange({ ...facets, hd: !facets.hd })}>
              HD only
            </Chip>
          </Group>

          {opts.categories.length > 0 && (
            <Group title="Categories">
              {opts.categories.map((c) => {
                const Icon = looks.get(c.key)?.icon
                return (
                  <Chip key={c.key} on={facets.categories.has(c.key)} count={c.count} onClick={() => onChange({ ...facets, categories: toggled(facets.categories, c.key) })}>
                    {Icon && <Icon aria-hidden className="size-3.5" />}
                    {c.label}
                  </Chip>
                )
              })}
            </Group>
          )}

          {opts.languages.length > 0 && (
            <Group title="Language">
              {opts.languages.map((l) => (
                <Chip key={l.key} on={facets.languages.has(l.key)} count={l.count} onClick={() => onChange({ ...facets, languages: toggled(facets.languages, l.key) })}>
                  <LanguageFlag code={l.key} />
                </Chip>
              ))}
            </Group>
          )}

          <p className="border-t border-border pt-3 text-xs text-muted-foreground">
            {opts.categories.length === 0 && opts.languages.length === 0
              ? "The program guide doesn't say what these channels show yet, so there is only quality to filter on."
              : `Categories and languages come from the program guide.${opts.unknown > 0 ? ` ${opts.unknown} ${opts.unknown === 1 ? 'channel isn’t' : 'channels aren’t'} in it, so a category or language filter leaves them out.` : ''}`}{' '}
            Showing {shown}.
          </p>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/** What is being filtered on, each as a chip that takes itself off, so it is always plain why the list is short. */
export function ActiveFilters({ facets, onChange }: { facets: Facets; onChange: (f: Facets) => void }) {
  if (facetCount(facets) === 0) return null
  const chip = (key: string, name: string, label: ReactNode, off: () => void) => (
    <li key={key}>
      <button
        type="button"
        onClick={off}
        aria-label={`Remove filter ${name}`}
        className="inline-flex min-h-7 items-center gap-1.5 rounded-full border border-primary/40 bg-primary/10 py-0.5 pr-1.5 pl-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {label}
        <X aria-hidden className="size-3.5 text-muted-foreground" />
      </button>
    </li>
  )
  return (
    <ul aria-label="Active filters" className="flex flex-wrap items-center gap-1.5">
      {facets.hd && chip('HD', 'HD only', 'HD only', () => onChange({ ...facets, hd: false }))}
      {[...facets.categories].map((c) => chip(c, c, c, () => onChange({ ...facets, categories: toggled(facets.categories, c) })))}
      {[...facets.languages].map((l) => chip(l, languageName(l), <LanguageFlag code={l} />, () => onChange({ ...facets, languages: toggled(facets.languages, l) })))}
      <li>
        <Button variant="ghost" size="xs" onClick={() => onChange(NO_FACETS)}>
          Clear all
        </Button>
      </li>
    </ul>
  )
}
