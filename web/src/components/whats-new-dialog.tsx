import { AlertTriangle, Check, ExternalLink } from 'lucide-react'
import { InlineMarkdown } from '@/components/inline-markdown'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { releaseDate, type Release } from '@/lib/changelog'

const RELEASES_URL = 'https://github.com/johnschult/antennarr/releases'

interface Props {
  open: boolean
  onClose: () => void
  /** The version running now, for the title. */
  version: string
  /** What to show, newest first. More than one when several releases went by. */
  releases: Release[]
  /** True when this opened by itself after an update, false when someone asked for it. */
  updated: boolean
}

/** "What's new": the release notes, shown once after an update and from the footer. */
export function WhatsNewDialog({ open, onClose, version, releases, updated }: Props) {
  const several = releases.length > 1
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>What's new in Antennarr {version}</DialogTitle>
          <DialogDescription>{updated ? (several ? 'Antennarr was updated. Here is what changed since you last looked.' : 'Antennarr was updated. Here is what changed.') : 'The release notes for this version.'}</DialogDescription>
        </DialogHeader>

        <div role="region" aria-label="Release notes" tabIndex={0} className="min-h-0 space-y-6 overflow-y-auto pr-1 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
          {releases.map((r) => (
            <section key={r.version} aria-label={`Release ${r.version}`} className="space-y-4">
              {(several || r.date) && (
                <h3 className="flex flex-wrap items-baseline gap-x-2 text-base font-semibold">
                  {several && <span>{r.version}</span>}
                  {r.date && <span className={several ? 'text-xs font-normal text-muted-foreground' : 'text-xs font-normal text-muted-foreground'}>{releaseDate(r.date)}</span>}
                </h3>
              )}
              {r.intro && <p className="text-muted-foreground">{r.intro}</p>}
              {r.sections.map((s, i) => {
                const heads = s.title === 'Read before updating'
                return (
                  <div key={s.title ?? i} className="space-y-2">
                    {s.title && (
                      <h4 className={heads ? 'flex items-center gap-1.5 text-sm font-semibold text-amber-800 dark:text-amber-300' : 'text-sm font-semibold'}>
                        {heads && <AlertTriangle aria-hidden className="size-4" />}
                        {s.title}
                      </h4>
                    )}
                    <ul className={heads ? 'list-disc space-y-2 rounded-lg border border-amber-500/30 bg-amber-500/10 py-3 pr-4 pl-8' : 'list-disc space-y-2 pl-5'}>
                      {s.items.map((item, j) => (
                        <li key={j} className="leading-relaxed text-foreground/90">
                          <InlineMarkdown text={item} />
                        </li>
                      ))}
                    </ul>
                  </div>
                )
              })}
            </section>
          ))}
        </div>

        <DialogFooter className="sm:justify-between">
          <a href={RELEASES_URL} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 self-center text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline">
            All releases <ExternalLink aria-hidden className="size-3" />
          </a>
          <Button onClick={onClose} autoFocus>
            <Check data-icon="inline-start" /> Got it!
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
