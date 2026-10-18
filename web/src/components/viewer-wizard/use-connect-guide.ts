import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useApp } from '@/hooks/app-context'
import { getNetwork } from '@/lib/api'
import { firewallRules, hostProblem, portOf, tunerAddress, type Firewall, type MediaServer, type Transport } from '@/lib/setup-guide'

/**
 * What the connect and media-server steps share: the address to give the viewer,
 * how they will reach it, whether there is a firewall, and the rules for the
 * viewer's addresses. `lookUp` is false until the person reaches the step that
 * needs this machine's own addresses, so nothing is read before then.
 */
export function useConnectGuide(viewerAddresses: string[], lookUp: boolean) {
  const { state } = useApp()
  const { devices } = state

  const [deviceName, setDeviceName] = useState(devices[0]?.name ?? '')
  // What the person typed, or null while the guide is choosing the address for them.
  const [typedHost, setTypedHost] = useState<string | null>(null)
  const [transport, setTransport] = useState<Transport>('tailscale')
  // The person's choices, or null while the guide goes by what it found on this machine.
  const [firewallGate, setFirewallGate] = useState<boolean | null>(null)
  const [firewallPick, setFirewallPick] = useState<Firewall | null>(null)
  const [media, setMedia] = useState<MediaServer>('plex')
  const network = useQuery({ queryKey: ['network'], queryFn: getNetwork, enabled: lookUp, staleTime: 60_000, retry: false, refetchOnWindowFocus: false })

  const device = devices.find((d) => d.name === deviceName) ?? devices[0]
  const port = device ? portOf(device.listen) : ''

  // This machine's own addresses, when the server could read them. The address
  // follows the connection chosen until the person types their own.
  const found = network.data?.addresses ?? []
  const tailscale = found.find((a) => a.kind === 'tailscale')
  const lan = found.find((a) => a.kind === 'lan')
  const suggested = (transport === 'tailscale' ? tailscale?.ip : transport === 'lan' ? lan?.ip : undefined) ?? window.location.hostname
  const host = typedHost ?? suggested

  // Which firewall is switched on here, if the server could tell. Only a firewall
  // seen to be on opens the section by itself; plain iptables might have no rules.
  const detected = network.data?.firewall
  const detectedTool = detected?.tool ? detected.tool : null
  const hasFirewall = firewallGate ?? detected?.active ?? false
  const firewall: Firewall = firewallPick ?? detectedTool ?? 'ufw'
  const problem = hostProblem(host)
  const address = tunerAddress(host, port)
  const rules = firewallRules(firewall, port, viewerAddresses)

  return {
    devices, device, port, setDeviceName,
    network, found, tailscale, host, setTypedHost, problem, address,
    transport, setTransport,
    detected, detectedTool, hasFirewall, setFirewallGate, firewall, setFirewallPick, rules,
    media, setMedia,
  }
}

export type ConnectGuide = ReturnType<typeof useConnectGuide>
