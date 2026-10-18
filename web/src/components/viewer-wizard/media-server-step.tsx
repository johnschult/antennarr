import { Block } from '@/components/setup/block'
import { CodeBlock } from '@/components/setup/code-block'
import { Steps } from '@/components/setup/steps'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MEDIA_SERVERS, mediaServerSteps, verifyCommands, type MediaServer } from '@/lib/setup-guide'
import type { ConnectGuide } from './use-connect-guide'

/** The selected tab is outlined in the accent colour and set apart from the others, so it can't be mistaken for them. */
const TAB =
  'gap-2 px-4 text-sm data-active:border-primary/60 data-active:bg-primary/15 data-active:font-semibold data-active:text-foreground ' +
  // In the dark theme the strip is mid-grey, so a faint tint doesn't stand out: make the selected tab darker than the strip, with a bright outline.
  'dark:data-active:border-primary dark:data-active:bg-background dark:data-active:ring-2 dark:data-active:ring-primary/70'

/** What to do in the viewer's media server, and how to check it worked. */
export function MediaServerStep({ g }: { g: ConnectGuide }) {
  return (
    <div className="space-y-6">
      <Block title="In their media server">
        {g.problem ? (
          <p className="text-sm text-muted-foreground">Fix the address on the previous step and the steps will appear here.</p>
        ) : (
          <Tabs value={g.media} onValueChange={(v) => g.setMedia(v as MediaServer)}>
            <TabsList className="w-full gap-1 p-1 group-data-horizontal/tabs:h-11">
              {MEDIA_SERVERS.map((m) => (
                <TabsTrigger key={m.id} value={m.id} className={TAB}>
                  <img src={`${import.meta.env.BASE_URL}${m.icon}`} alt="" aria-hidden="true" className="size-5 rounded-sm" draggable={false} />
                  {m.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {MEDIA_SERVERS.map((m) => (
              <TabsContent key={m.id} value={m.id} className="pt-3">
                <Steps steps={mediaServerSteps(m.id, g.address)} />
              </TabsContent>
            ))}
          </Tabs>
        )}
      </Block>

      <Block title="Check that it works" description="Run these on the viewer's media server. The first should print JSON that includes your tuner's name and an address that is yours, not the tuner's own.">
        {g.problem ? <p className="text-sm text-muted-foreground">Fix the address on the previous step first.</p> : <CodeBlock label="check commands" lines={verifyCommands(g.address)} />}
      </Block>
    </div>
  )
}
