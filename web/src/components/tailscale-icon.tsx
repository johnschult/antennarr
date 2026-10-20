import { cn } from '@/lib/utils'

/** The Tailscale logo (from Dashboard Icons): black on a light page, turned white on a dark one. */
export function TailscaleIcon({ className }: { className?: string }) {
  return <img src={`${import.meta.env.BASE_URL}tailscale-icon.svg`} alt="" aria-hidden="true" className={cn('size-6 dark:invert', className)} draggable={false} />
}
