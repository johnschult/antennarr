import { useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { Link } from 'react-router'
import { CodeBlock } from '@/components/setup/code-block'
import { Choice } from '@/components/setup/choice'
import { Steps } from '@/components/setup/steps'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useApp } from '@/hooks/app-context'
import { getNetwork } from '@/lib/api'
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

/** The selected tab is outlined in the accent colour and set apart from the others, so it can't be mistaken for them. */
const TAB =
  'gap-2 px-4 text-sm data-active:border-primary/60 data-active:bg-primary/15 data-active:font-semibold data-active:text-foreground ' +
  // In the dark theme the strip is mid-grey, so a faint tint doesn't stand out: make the selected tab darker than the strip, with a bright outline.
  'dark:data-active:border-primary dark:data-active:bg-background dark:data-active:ring-2 dark:data-active:ring-primary/70'

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
  // What the person typed, or null while the page is choosing the address for them.
  const [typedHost, setTypedHost] = useState<string | null>(null)
  const [viewerName, setViewerName] = useState(viewers[0]?.name ?? '')
  const [transport, setTransport] = useState<Transport>('tailscale')
  const network = useQuery({ queryKey: ['network'], queryFn: getNetwork, staleTime: 60_000, retry: false, refetchOnWindowFocus: false })
  // The person's choices, or null while the page goes by what it found on this machine.
  const [firewallGate, setFirewallGate] = useState<boolean | null>(null)
  const [firewallPick, setFirewallPick] = useState<Firewall | null>(null)
  const [media, setMedia] = useState<MediaServer>('plex')

  const device = devices.find((d) => d.name === deviceName) ?? devices[0]
  const viewer = viewers.find((v) => v.name === viewerName) ?? viewers[0]
  const port = device ? portOf(device.listen) : ''

  // This machine's own addresses, when the server could read them. The address
  // follows the connection chosen in step 3 until the person types their own.
  const found = network.data?.addresses ?? []
  const tailscale = found.find((a) => a.kind === 'tailscale')
  const lan = found.find((a) => a.kind === 'lan')
  const suggested = (transport === 'tailscale' ? tailscale?.ip : transport === 'lan' ? lan?.ip : undefined) ?? window.location.hostname
  const host = typedHost ?? suggested

  // Which firewall is switched on here, if the server could tell. Only a firewall
  // seen to be on opens the step by itself; plain iptables might have no rules.
  const detected = network.data?.firewall
  const detectedTool = detected?.tool ? detected.tool : null
  const hasFirewall = firewallGate ?? detected?.active ?? false
  const firewall: Firewall = firewallPick ?? detectedTool ?? 'ufw'
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
          <Input id="setup-host" value={host} onChange={(e) => setTypedHost(e.target.value)} spellCheck={false} autoComplete="off" aria-invalid={problem !== null} className="max-w-sm font-mono" />
          {problem && (
            <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
              {problem}
            </p>
          )}
        </div>
        {found.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-xs" role="group" aria-label="Addresses of this machine">
            <span className="text-muted-foreground">This machine is at</span>
            {found.map((a) => (
              <button
                key={a.ip}
                type="button"
                aria-pressed={host === a.ip}
                onClick={() => setTypedHost(a.ip)}
                className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card/70 px-2.5 py-1 font-mono hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/10"
              >
                {a.ip}
                <span className="font-sans text-muted-foreground">{a.kind === 'tailscale' ? 'Tailscale' : 'LAN'}</span>
              </button>
            ))}
          </div>
        )}
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
        <Choice
          label="Connection"
          value={transport}
          onChange={setTransport}
          options={TRANSPORTS.map((t) => (t.id === 'tailscale' && tailscale ? { ...t, hint: `Running on this machine (${tailscale.ip})` } : t))}
        />
        {transport === 'tailscale' && network.isSuccess && !tailscale && (
          <p role="status" className="text-xs text-muted-foreground">
            Tailscale wasn't found running on this machine. If Antennarr runs in a container it can't see the host's network, so use the steps below to find the address.
          </p>
        )}
        {/* Wait for the check: until it finishes we can't tell which Tailscale steps apply. */}
        {!(transport === 'tailscale' && network.isPending) && <Steps steps={transportSteps(transport, tailscale?.ip)} />}
      </Block>

      <Block title="4. On this machine">
        <div className="flex items-center gap-3">
          <Switch id="setup-firewall" checked={hasFirewall} onCheckedChange={setFirewallGate} />
          <Label htmlFor="setup-firewall" className="font-normal">
            This machine has a firewall that blocks port {port}
          </Label>
        </div>
        {detected && detectedTool && (
          <p role="status" className="text-xs text-muted-foreground">
            {detected.active
              ? `Antennarr found ${detectedTool} switched on here.`
              : `${detectedTool} looks to be in use here, but Antennarr can't see its rules, so it can't tell whether port ${port} is blocked.`}
          </p>
        )}
        {hasFirewall && (
          <>
            <p className="text-xs text-muted-foreground">Antennarr can't read your firewall's rules. These add an allow for the viewer's address, and only that.</p>
            <Choice label="Firewall" value={firewall} onChange={setFirewallPick} options={FIREWALLS} />
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
          </>
        )}
      </Block>

      <Block title="5. In their media server">
        {problem ? (
          <p className="text-sm text-muted-foreground">Fix the address in step 1 and the steps will appear here.</p>
        ) : (
          <Tabs value={media} onValueChange={(v) => setMedia(v as MediaServer)}>
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
