import { Cpu, Gauge, Server, SlidersHorizontal, type LucideIcon } from 'lucide-react'

export type TunerTab = 'overview' | 'device' | 'firmware' | 'settings'

export const TUNER_TABS: { id: TunerTab; label: string; icon: LucideIcon }[] = [
  { id: 'overview', label: 'Overview', icon: Gauge },
  { id: 'device', label: 'Device', icon: Server },
  { id: 'firmware', label: 'Firmware', icon: Cpu },
  { id: 'settings', label: 'Settings', icon: SlidersHorizontal },
]

export const isTunerTab = (s: string | undefined): s is TunerTab => TUNER_TABS.some((t) => t.id === s)

/** Where a tuner's page, or one of its tabs, is. The Overview is the page itself. */
export function tunerPath(name: string, tab: TunerTab = 'overview'): string {
  const base = `/tuner/${encodeURIComponent(name)}`
  return tab === 'overview' ? base : `${base}/${tab}`
}
