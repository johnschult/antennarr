import type { NetworkAddress } from './types'

/** One address a viewer might reach this Yagi at, and the start of the link that goes with it. */
export interface LinkChoice {
  /** "http://192.168.1.5:9244" */
  origin: string
  label: string
}

const onThisMachineOnly = (host: string) => host === 'localhost' || host === '127.0.0.1' || host === '[::1]' || host.endsWith('.localhost')

/** Whether an address only works on the machine it is typed on, so a link that starts with it is no good to send. */
export function isLocalOrigin(origin: string): boolean {
  try {
    return onThisMachineOnly(new URL(origin).hostname)
  } catch {
    return false
  }
}

/**
 * The addresses to offer for the start of an invite link: the one this page was opened at, and each way the machine
 * can be reached (its Tailscale and LAN addresses, on the same port as this page). The page's own is first unless it
 * is only good on this machine, when a real address comes first.
 */
export function linkChoices(here: { origin: string; port: string; protocol: string }, addresses: NetworkAddress[]): LinkChoice[] {
  const found: LinkChoice[] = addresses.map((a) => ({
    origin: `${here.protocol}//${a.ip.includes(':') ? `[${a.ip}]` : a.ip}${here.port ? `:${here.port}` : ''}`,
    label: `${a.kind === 'tailscale' ? 'Tailscale' : 'Network'}: ${a.ip}`,
  }))
  const own: LinkChoice = { origin: here.origin, label: `This page: ${here.origin.replace(/^https?:\/\//, '')}` }
  const order = (a: LinkChoice, b: LinkChoice) => Number(a.label.startsWith('Network')) - Number(b.label.startsWith('Network'))
  const real = found.sort(order)
  return isLocalOrigin(here.origin) ? [...real, own] : [own, ...real.filter((c) => c.origin !== here.origin)]
}
