import { useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { CodeBlock } from '@/components/setup/code-block'
import { Choice } from '@/components/setup/choice'
import { Steps } from '@/components/setup/steps'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'
import {
  FIREWALLS,
  MEDIA_SERVERS,
  TRANSPORTS,
  firewallRules,
  hostProblem,
  mediaServerSteps,
  portOf,
  transportSteps,
  tunerAddress,
  verifyCommands,
  type Firewall,
  type MediaServer,
  type Transport,
} from '@/lib/setup-guide'

function Block({ title, description, children }: { title: string; description?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

/** Copy-paste setup for a viewer: how their server reaches this one, and how to add the tuner to their media server. */
export function SetupPage() {
  const { state } = useApp()
  const { devices, viewers } = state
  const several = devices.length > 1

  const [deviceName, setDeviceName] = useState(devices[0]?.name ?? '')
  const [host, setHost] = useState(window.location.hostname)
  const [viewerName, setViewerName] = useState(viewers[0]?.name ?? '')
  const [transport, setTransport] = useState<Transport>('tailscale')
  const [firewall, setFirewall] = useState<Firewall>('ufw')
  const [media, setMedia] = useState<MediaServer>('plex')

  const device = devices.find((d) => d.name === deviceName) ?? devices[0]
  const viewer = viewers.find((v) => v.name === viewerName) ?? viewers[0]
  const port = device ? portOf(device.listen) : ''
  const problem = hostProblem(host)
  const address = tunerAddress(host, port)
  const rules = firewallRules(firewall, port, viewer?.addresses ?? [])

  return (
    <div className="max-w-3xl space-y-8">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Setup guide</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Pick how a viewer will reach Antennarr and which media server they use, and this page writes out the steps with your addresses filled in. Nothing here runs by itself:
          you copy each step.
        </p>
      </div>

      <Block title="1. The address">
        <div className="space-y-2">
          <Label htmlFor="setup-host">Address viewers will use to reach this machine</Label>
          <Input id="setup-host" value={host} onChange={(e) => setHost(e.target.value)} spellCheck={false} autoComplete="off" aria-invalid={problem !== null} className="max-w-sm font-mono" />
          {problem && (
            <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
              {problem}
            </p>
          )}
        </div>
        {several && (
          <Choice
            label="Tuner"
            value={device?.name ?? ''}
            onChange={setDeviceName}
            options={devices.map((d) => ({ id: d.name, label: deviceTitle(d), hint: `port ${portOf(d.listen)}` }))}
          />
        )}
        {!problem && <CodeBlock label="tuner address" lines={[address]} />}
      </Block>

      <Block
        title="2. Who is it for?"
        description="Antennarr only serves addresses on the Viewers page, so the viewer needs an entry there with the address their media server connects from."
      >
        {viewers.length === 0 ? (
          <p className="rounded-xl border border-dashed border-border px-4 py-5 text-sm text-muted-foreground">
            No viewers yet.{' '}
            <Link to="/viewers" className="underline underline-offset-2 hover:text-foreground">
              Add one
            </Link>{' '}
            so their address is allowed.
          </p>
        ) : (
          <>
            <Choice
              label="Viewer"
              value={viewer.name}
              onChange={setViewerName}
              options={viewers.map((v) => ({ id: v.name, label: v.name, hint: v.addresses.join(', ') }))}
            />
          </>
        )}
      </Block>

      <Block title="3. How will they reach it?">
        <Choice label="Connection" value={transport} onChange={setTransport} options={TRANSPORTS} />
        <Steps steps={transportSteps(transport, problem ? '<address>' : address)} />
      </Block>

      <Block
        title="4. On this machine"
        description={`Only needed if the machine running Antennarr has a firewall that blocks port ${port}. Allow the viewer's address, and only that.`}
      >
        <Choice label="Firewall" value={firewall} onChange={setFirewall} options={FIREWALLS} />
        {rules.add.length === 0 ? (
          <p className="text-sm text-muted-foreground">{viewers.length === 0 ? 'Add a viewer to get their firewall rules.' : 'This viewer has no addresses to allow.'}</p>
        ) : (
          <div className="space-y-3">
            <CodeBlock label="firewall rules" lines={rules.add} />
            <details className="text-sm">
              <summary className="cursor-pointer text-muted-foreground hover:text-foreground">Undo these rules</summary>
              <div className="mt-2">
                <CodeBlock label="undo firewall rules" lines={rules.undo} />
              </div>
            </details>
          </div>
        )}
        {rules.skipped.length > 0 && (
          <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
            Left out because they aren't a plain IP address or network: {rules.skipped.join(', ')}
          </p>
        )}
      </Block>

      <Block title="5. In their media server">
        {problem ? (
          <p className="text-sm text-muted-foreground">Fix the address in step 1 and the steps will appear here.</p>
        ) : (
          <Tabs value={media} onValueChange={(v) => setMedia(v as MediaServer)}>
            <TabsList>
              {MEDIA_SERVERS.map((m) => (
                <TabsTrigger key={m.id} value={m.id}>
                  {m.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {MEDIA_SERVERS.map((m) => (
              <TabsContent key={m.id} value={m.id} className="pt-3">
                <Steps steps={mediaServerSteps(m.id, address)} />
              </TabsContent>
            ))}
          </Tabs>
        )}
      </Block>

      <Block title="6. Check that it works" description="Run these on the viewer's media server. The first should print JSON that includes your tuner's name and an address that is yours, not the tuner's own.">
        {problem ? <p className="text-sm text-muted-foreground">Fix the address in step 1 first.</p> : <CodeBlock label="check commands" lines={verifyCommands(address)} />}
      </Block>
    </div>
  )
}
