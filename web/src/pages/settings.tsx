import { Bell, Gauge, KeyRound, ShieldCheck } from 'lucide-react'
import { Link } from 'react-router'

import { EditRow } from '@/components/settings/edit-row'
import { NotificationRows } from '@/components/settings/notification-rows'
import { PlexRows } from '@/components/settings/plex-rows'
import { Row, Section } from '@/components/settings/section'
import { SwitchRow } from '@/components/settings/switch-row'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { deviceTitle } from '@/lib/device'
import { parseAddresses } from '@/lib/format'
import type { DevicePatch } from '@/lib/types'

const wholeNumber = (text: string) =>
  text.trim() === '' || !Number.isInteger(Number(text)) || Number(text) < 0 ? 'Enter a whole number, 0 or more.' : null

export function SettingsPage() {
  const { state } = useApp()
  const { settings, devices, editable } = state
  const { device, settings: update } = useEdits()
  const several = devices.length > 1

  const saveDevice = (name: string, patch: DevicePatch) => device.mutateAsync({ name, patch })

  return (
    <div className="max-w-3xl space-y-8">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>

      {devices.map((d) => (
        <Section
          key={d.name}
          icon={Gauge}
          title={several ? `Limits: ${deviceTitle(d)}` : 'Limits'}
          description={`How much of ${several ? 'this tuner' : (d.label ?? 'your tuner')} viewers may use. ${d.tuner.friendlyName ?? 'HDHomeRun'} · ${d.tuner.tunerCount} tuners.`}
        >
          <EditRow
            key={`reserve-${d.name}-${d.ownerReserve}`}
            kind="number"
            min={0}
            max={d.tuner.tunerCount}
            id={`setting-${d.name}-ownerReserve`}
            title="Tuners reserved for you"
            description="Tuners always held back for your own TVs and apps. Viewers are refused instead of taking the last ones."
            initial={String(d.ownerReserve)}
            validate={wholeNumber}
            disabled={!editable}
            onSave={(text) => saveDevice(d.name, { ownerReserve: Number(text) })}
          />
          <EditRow
            key={`cap-${d.name}-${d.sharedCap}`}
            kind="number"
            min={0}
            max={d.tuner.tunerCount}
            id={`setting-${d.name}-sharedCap`}
            title="Max tuners for viewers"
            description="The most tuners all viewers together may use. 0 means no cap beyond what you reserve."
            initial={String(d.sharedCap)}
            validate={wholeNumber}
            disabled={!editable}
            onSave={(text) => saveDevice(d.name, { sharedCap: Number(text) })}
          />
        </Section>
      ))}

      <Section title="Access" icon={KeyRound}>
        <EditRow
          key={`admin-${settings.admin.join(',')}`}
          kind="lines"
          id="setting-admin"
          title="Admin addresses"
          description="Addresses allowed to open this page, besides the machine Antennarr runs on. One per line: an IP or a range like 192.168.1.0/24. Keep viewers out of these ranges: anyone listed here can change settings. Take care not to lock yourself out."
          initial={settings.admin.join('\n')}
          placeholder="100.64.0.1"
          disabled={!editable}
          onSave={(text) => update.mutateAsync({ admin: parseAddresses(text) })}
        />
        <PlexRows />
        <Row title="Who can use your tuners" description="Only the addresses of the viewers you add. With no viewers, nobody can.">
          <Link to="/viewers" className="text-sm font-medium underline underline-offset-2 hover:text-foreground">
            {settings.mode === 'no-viewers' ? 'No viewers yet' : 'Viewers only'}
          </Link>
        </Row>
      </Section>

      <Section
        title="Notifications"
        icon={Bell}
        description="Messages about firmware, tuners and viewers, to your phone or another service."
      >
        <NotificationRows />
      </Section>

      <Section title="Privacy and updates" icon={ShieldCheck}>
        <SwitchRow
          id="setting-firmwareCheck"
          title="Check for new tuner firmware"
          description="Reads SiliconDust's public firmware changelog once every 12 hours. Nothing about you or your tuners is sent. Antennarr only tells you about new releases; it never installs them."
          checked={settings.firmwareCheck}
          disabled={!editable}
          onChange={(next) => update.mutateAsync({ firmwareCheck: next })}
        />
        <SwitchRow
          id="setting-guide"
          title="Show what's on"
          description="Shows each watched channel's logo and current programme on its tile. Antennarr reads SiliconDust's programme guide for your tuner every 6 hours, which sends SiliconDust your tuner's guide key. Nothing about your viewers is sent, and logos are served by Antennarr, not loaded by your browser."
          checked={settings.guide}
          disabled={!editable}
          onChange={(next) => update.mutateAsync({ guide: next })}
        />
      </Section>
    </div>
  )
}
