import type { ChannelType, NotifyChannel } from './types'

export const CHANNEL_TYPES: { id: ChannelType; label: string; hint: string }[] = [
  { id: 'ntfy', label: 'ntfy', hint: 'Push to your phone, from ntfy.sh or your own server' },
  { id: 'pushover', label: 'Pushover', hint: 'Push through the Pushover app' },
  { id: 'webhook', label: 'Webhook', hint: 'A JSON POST to any address' },
]

export const NTFY_SERVER = 'https://ntfy.sh'

/** One line about where a channel sends: the service, and the server and topic or the host. */
export function channelSummary(c: NotifyChannel): string {
  const service = CHANNEL_TYPES.find((t) => t.id === c.type)?.label ?? c.type
  if (c.type === 'ntfy') return `${service} · ${(c.url ?? '').replace(/^https?:\/\//, '').replace(/\/$/, '')}/${c.topic ?? ''}`
  if (c.type === 'webhook') return `${service} · ${c.host ?? ''}`
  return service
}
