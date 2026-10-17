import { useState } from 'react'
import { cn } from '@/lib/utils'

/** A channel's logo, or nothing at all if it won't load. Served by Antennarr, so no one is asked to contact SiliconDust. */
export function ChannelLogo({ src, name, className }: { src: string; name: string; className?: string }) {
  const [failed, setFailed] = useState(false)
  if (failed) return null
  return <img src={src} alt={`${name} logo`} onError={() => setFailed(true)} className={cn('shrink-0 rounded-md object-contain', className)} draggable={false} />
}
