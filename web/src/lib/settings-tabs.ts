import { Bell, KeyRound, ScrollText, ShieldCheck, type LucideIcon } from 'lucide-react'

export type SettingsTab = 'access' | 'notifications' | 'privacy' | 'logs'

export const SETTINGS_TABS: { id: SettingsTab; label: string; icon: LucideIcon }[] = [
  { id: 'access', label: 'Access', icon: KeyRound },
  { id: 'notifications', label: 'Notifications', icon: Bell },
  { id: 'privacy', label: 'Privacy', icon: ShieldCheck },
  { id: 'logs', label: 'Logs', icon: ScrollText },
]

export const isSettingsTab = (s: string | undefined): s is SettingsTab => SETTINGS_TABS.some((t) => t.id === s)

/** Where a settings tab is. Access is the page itself. */
export const settingsPath = (tab: SettingsTab = 'access'): string => (tab === 'access' ? '/settings' : `/settings/${tab}`)
