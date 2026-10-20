import { Hd } from 'lucide-react'
import { cn } from '@/lib/utils'

/** The mark for a high-definition channel: the HD icon, with "HD" for a screen reader and as its tooltip. */
export function HdMark({ className }: { className?: string }) {
  return (
    <span title="High definition" className={cn('inline-flex shrink-0 items-center text-muted-foreground', className)}>
      <Hd aria-hidden className="size-5" />
      <span className="sr-only">HD</span>
    </span>
  )
}
