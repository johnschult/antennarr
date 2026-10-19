import { useState } from 'react'
import { knownTone, toneOfImage, type LogoTone } from '@/lib/logo-tone'
import { cn } from '@/lib/utils'

// A thin outline in the one color that makes the artwork stand out, so a logo never vanishes into the page:
// white lettering gets a dark outline in the light theme (and none in the dark one, where it shows), and dark
// artwork gets a light outline in the dark theme. A logo with a mix of colors gets nothing.
const dark = '[filter:drop-shadow(0_0_1px_rgb(0_0_0/0.8))_drop-shadow(0_0_1px_rgb(0_0_0/0.8))]'
const light = '[filter:drop-shadow(0_0_1px_rgb(255_255_255/0.85))_drop-shadow(0_0_1px_rgb(255_255_255/0.85))]'
const outline: Record<LogoTone, string> = {
  light: `${dark} dark:[filter:none]`,
  dark: `dark:${light}`,
  mixed: '',
}

const sizes = { md: 'h-8 max-w-20', sm: 'h-6 max-w-12' }

/**
 * A channel's logo, outlined if it needs it, or nothing at all if it won't load. Served by Yagi, so no
 * one is asked to contact SiliconDust.
 */
export function ChannelLogo({ src, name, size = 'md', className }: { src: string; name: string; size?: keyof typeof sizes; className?: string }) {
  const [failed, setFailed] = useState(false)
  const [tone, setTone] = useState<LogoTone>(() => knownTone(src) ?? 'mixed')
  if (failed) return null
  return (
    <img
      src={src}
      alt={`${name} logo`}
      data-tone={tone}
      onLoad={(e) => setTone(toneOfImage(src, e.currentTarget))}
      onError={() => setFailed(true)}
      className={cn('shrink-0 rounded-sm object-contain', sizes[size], outline[tone], className)}
      draggable={false}
    />
  )
}
