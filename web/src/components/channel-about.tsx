import { Check, EyeOff, Info, Languages, RadioReceiver, Star, Tags, Tv } from 'lucide-react'
import type { ComponentType, ReactNode } from 'react'
import { LanguageFlag } from '@/components/language-flag'
import { Badge } from '@/components/ui/badge'
import { Drawer, DrawerContent, DrawerDescription, DrawerTitle, DrawerTrigger } from '@/components/ui/drawer'
import { Popover, PopoverContent, PopoverDescription, PopoverTitle, PopoverTrigger } from '@/components/ui/popover'
import { useIsPhone } from '@/hooks/use-media-query'
import { categoryLooks } from '@/lib/channel-about'
import type { ChannelState, TunerChannel } from '@/lib/types'
import { cn } from '@/lib/utils'

const STATE: Record<ChannelState, { label: string; icon: typeof Check; tone: string }> = {
  enabled: { label: 'In the lineup', icon: Check, tone: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300' },
  favorite: { label: 'Favorite', icon: Star, tone: 'bg-amber-500/15 text-amber-700 dark:text-amber-300' },
  disabled: { label: 'Disabled', icon: EyeOff, tone: 'bg-red-500/15 text-red-700 dark:text-red-300' },
}

/** One fact: a picture for what kind of fact it is, and then the fact. */
function Fact({ icon: Icon, label, children }: { icon: typeof Tv; label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[1rem_1fr] gap-x-3">
      <Icon aria-hidden className="mt-px size-4 text-muted-foreground" />
      <div className="min-w-0">
        <div className="text-[11px] tracking-wide text-muted-foreground uppercase">{label}</div>
        <div className="mt-1 min-h-5 text-sm">{children}</div>
      </div>
    </div>
  )
}

const TRIGGER_CLASS =
  '-mr-1 inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-muted data-popup-open:text-foreground data-[state=open]:bg-muted data-[state=open]:text-foreground'

/** What is known about a channel: the same on a popover and on a phone's sheet. */
type TextPart = ComponentType<{ className?: string; children?: ReactNode }>

function AboutBody({ channel, Title, Description }: { channel: TunerChannel; Title: TextPart; Description: TextPart }) {
  const a = channel.about
  const state = STATE[channel.state]
  const looks = categoryLooks(a?.categories)
  const tuner = [channel.video, channel.audio].filter(Boolean).join(' · ')
  const hasGuide = !!a && (!!a.network || !!a.languages?.length || looks.length > 0)
  return (
    <>
      <div data-testid="channel-about" className="space-y-4 p-4">
        <div className="flex items-start gap-3">
          <span className="rounded-lg bg-muted px-2.5 py-1 font-mono text-xl leading-none font-bold tabular-nums">{channel.number}</span>
          <div className="min-w-0 flex-1">
            <Title className="truncate">{channel.name || 'Unnamed channel'}</Title>
            <Description className="sr-only">What is known about this channel</Description>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              <Badge className={cn('gap-1', state.tone)}>
                <state.icon aria-hidden className="size-3" /> {state.label}
              </Badge>
              {channel.hd && <Badge variant="secondary">HD</Badge>}
              {channel.drm && <Badge variant="secondary">DRM</Badge>}
            </div>
          </div>
        </div>

        {a?.network && (
          <Fact icon={Tv} label="Network">
            {a.network}
          </Fact>
        )}
        {!!a?.languages?.length && (
          <Fact icon={Languages} label={a.languages.length > 1 ? 'Languages' : 'Language'}>
            <span className="flex flex-wrap gap-x-4 gap-y-1">
              {a.languages.map((l) => (
                <LanguageFlag key={l} code={l} />
              ))}
            </span>
          </Fact>
        )}
        {looks.length > 0 && (
          <Fact icon={Tags} label="Categories">
            <ul aria-label="Categories" className="flex flex-wrap gap-1.5">
              {looks.map((l) => (
                <li key={l.label} className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/50 py-1 pr-2.5 pl-2 text-xs">
                  <l.icon aria-hidden className="size-3.5 text-muted-foreground" />
                  {l.label}
                </li>
              ))}
            </ul>
          </Fact>
        )}
        {tuner && (
          <Fact icon={RadioReceiver} label="From the tuner">
            {tuner}
          </Fact>
        )}
        {!hasGuide && <p className="text-sm text-muted-foreground">The program guide doesn't say anything about this channel.</p>}
      </div>
      {hasGuide && <div className="border-t border-border px-4 py-2 text-[11px] text-muted-foreground">Metadata provided by SiliconDust</div>}
    </>
  )
}

/** The i beside a channel's name: opens what is known about the channel, as a popover, or as a sheet from the bottom on a phone. */
export function ChannelAbout({ channel }: { channel: TunerChannel }) {
  const phone = useIsPhone()
  const label = `About ${channel.number} ${channel.name}`
  if (phone) {
    return (
      <Drawer>
        <DrawerTrigger aria-label={label} title="About this channel" className={TRIGGER_CLASS}>
          <Info aria-hidden className="size-4" />
        </DrawerTrigger>
        <DrawerContent aria-describedby={undefined} className="overflow-y-auto pb-[env(safe-area-inset-bottom)]">
          <AboutBody channel={channel} Title={DrawerTitle} Description={DrawerDescription} />
        </DrawerContent>
      </Drawer>
    )
  }
  return (
    <Popover>
      <PopoverTrigger aria-label={label} title="About this channel" className={TRIGGER_CLASS}>
        <Info aria-hidden className="size-4" />
      </PopoverTrigger>
      <PopoverContent>
        <AboutBody channel={channel} Title={PopoverTitle} Description={PopoverDescription} />
      </PopoverContent>
    </Popover>
  )
}
