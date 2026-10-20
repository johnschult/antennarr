import { Block } from '@/components/setup/block'
import { Choice } from '@/components/setup/choice'
import { CodeBlock } from '@/components/setup/code-block'
import { Steps } from '@/components/setup/steps'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { deviceTitle } from '@/lib/device'
import { FIREWALLS, TRANSPORTS, portOf, transportSteps, yagiTransport } from '@/lib/setup-guide'
import type { ConnectGuide } from './use-connect-guide'

/** How the viewer's media server will reach this machine: the address, the way they connect, and any firewall here. */
/** The addresses that belong to the way chosen, each saying what it is, so the list is short and nothing is mixed up. */
function addressesFor(g: ConnectGuide): { value: string; tag: string }[] {
  switch (g.transport) {
    case 'yagi': {
      const y = g.yagi
      if (!y || y.state !== 'running') return []
      return [...(y.dnsName ? [{ value: y.dnsName, tag: 'name' }] : []), ...(y.addresses ?? []).map((ip) => ({ value: ip, tag: 'address' }))]
    }
    case 'tailscale':
      return g.found.filter((a) => a.kind === 'tailscale').map((a) => ({ value: a.ip, tag: 'Tailscale' }))
    case 'lan':
      return g.found.filter((a) => a.kind === 'lan').map((a) => ({ value: a.ip, tag: 'LAN' }))
    default:
      return []
  }
}

/** How the viewer's media server will reach this machine: the way they connect, the address that goes with it, and any firewall here. */
export function ConnectStep({ g }: { g: ConnectGuide }) {
  const several = g.devices.length > 1
  const { network, tailscale, yagi, problem } = g
  const options = [
    ...(yagi ? [yagiTransport(yagi)] : []),
    ...TRANSPORTS.map((t) =>
      t.id === 'tailscale'
        ? { ...t, label: yagi ? 'Tailscale on this machine' : t.label, hint: tailscale ? `Running on this machine (${tailscale.ip})` : t.hint }
        : t,
    ),
  ]
  const here = addressesFor(g)
  const whose = g.transport === 'yagi' ? 'Yagi\u2019s own Tailscale is at' : g.transport === 'tailscale' ? 'Tailscale on this machine is at' : 'This machine is at'

  return (
    <div className="space-y-6">
      <Block title="How will they reach it?">
        <Choice label="Connection" value={g.transport} onChange={g.setTransport} options={options} />
        {g.transport === 'tailscale' && network.isSuccess && !tailscale && (
          <p role="status" className="text-xs text-muted-foreground">
            Tailscale wasn't found running on this machine. If Yagi runs in a container it can't see the host's network, so use the steps below to find the address.
          </p>
        )}
        {/* Wait for the check: until it finishes we can't tell which Tailscale steps apply. */}
        {!(g.transport === 'tailscale' && network.isPending) && <Steps steps={transportSteps(g.transport, tailscale?.ip, yagi ?? undefined)} />}
      </Block>

      <Block title="The address">
        <div className="space-y-2">
          <Label htmlFor="setup-host">Address viewers will use to reach this machine</Label>
          <Input id="setup-host" value={g.host} onChange={(e) => g.setTypedHost(e.target.value)} spellCheck={false} aria-invalid={problem !== null} className="max-w-sm font-mono" />
          {problem && g.transport !== 'yagi' && (
            <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
              {problem}
            </p>
          )}
        </div>
        {here.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-xs" role="group" aria-label="Addresses for this way">
            <span className="text-muted-foreground">{whose}</span>
            {here.map((a) => (
              <button
                key={a.value}
                type="button"
                aria-pressed={g.host === a.value}
                onClick={() => g.setTypedHost(a.value)}
                className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card/70 px-2.5 py-1 font-mono hover:bg-muted aria-pressed:border-primary aria-pressed:bg-primary/10"
              >
                {a.value}
                <span className="font-sans text-muted-foreground">{a.tag}</span>
              </button>
            ))}
          </div>
        )}
        {several && (
          <Choice
            label="Tuner"
            value={g.device?.name ?? ''}
            onChange={g.setDeviceName}
            options={g.devices.map((d) => ({ id: d.name, label: deviceTitle(d), hint: `port ${portOf(d.listen)}` }))}
          />
        )}
        {!problem && <CodeBlock label="tuner address" lines={[g.address]} />}
      </Block>

      {/* Yagi's own Tailscale node is reached through the tailnet, not this machine's network, so its firewall has nothing to do with it. */}
      {g.transport !== 'yagi' && <FirewallBlock g={g} />}
    </div>
  )
}

/** Whether this machine has a firewall that blocks the port, and the rules that let the viewer through it. */
function FirewallBlock({ g }: { g: ConnectGuide }) {
  return (
    <Block title="On this machine">
    <div className="flex items-center gap-3">
      <Switch id="setup-firewall" checked={g.hasFirewall} onCheckedChange={g.setFirewallGate} />
      <Label htmlFor="setup-firewall" className="font-normal">
        This machine has a firewall that blocks port {g.port}
      </Label>
    </div>
      {g.detected && g.detectedTool && (
      <p role="status" className="text-xs text-muted-foreground">
        {g.detected.active
          ? `Yagi found ${g.detectedTool} switched on here.`
          : `${g.detectedTool} looks to be in use here, but Yagi can't see its rules, so it can't tell whether port ${g.port} is blocked.`}
      </p>
    )}
      {g.hasFirewall && (
      <>
        <p className="text-xs text-muted-foreground">Yagi can't read your firewall's rules. These add an allow for the viewer's address, and only that.</p>
        <Choice label="Firewall" value={g.firewall} onChange={g.setFirewallPick} options={FIREWALLS} />
        {g.rules.add.length === 0 ? (
          <p className="text-sm text-muted-foreground">This viewer has no addresses to allow.</p>
        ) : (
          <div className="space-y-3">
            <CodeBlock label="firewall rules" lines={g.rules.add} />
            <details className="text-sm">
              <summary className="cursor-pointer text-muted-foreground hover:text-foreground">Undo these rules</summary>
              <div className="mt-2">
                <CodeBlock label="undo firewall rules" lines={g.rules.undo} />
              </div>
            </details>
          </div>
        )}
        {g.rules.skipped.length > 0 && (
          <p role="alert" className="text-xs text-amber-700 dark:text-amber-300">
            Left out because they aren't a plain IP address or network: {g.rules.skipped.join(', ')}
          </p>
        )}
      </>
      )}
    </Block>
  )
}
