import { EditRow } from '@/components/settings/edit-row'
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
          title={several ? deviceTitle(d) : (d.label ?? 'Tuner')}
          description={`${d.tuner.friendlyName ?? 'HDHomeRun'} · ${d.tuner.tunerCount} tuners · listening on ${d.listen}`}
        >
          <EditRow
            key={`label-${d.name}-${d.label ?? ''}`}
            kind="text"
            maxLength={60}
            id={`setting-${d.name}-label`}
            title="Name"
            description="What this tuner is called here, and to the media servers that use it."
            initial={d.label ?? ''}
            placeholder={d.tuner.friendlyName ?? d.name}
            disabled={!editable}
            onSave={(text) => saveDevice(d.name, { label: text.trim() })}
          />
          <EditRow
            key={`reserve-${d.name}-${d.ownerReserve}`}
            kind="number"
            min={0}
            max={d.tuner.tunerCount}
            id={`setting-${d.name}-ownerReserve`}
            title="Keep free for you"
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
            title="Most for viewers"
            description="The most tuners all viewers together may use. 0 means no cap beyond what you keep free."
            initial={String(d.sharedCap)}
            validate={wholeNumber}
            disabled={!editable}
            onSave={(text) => saveDevice(d.name, { sharedCap: Number(text) })}
          />
        </Section>
      ))}

      <Section title="Access">
        <EditRow
          key={`admin-${settings.admin.join(',')}`}
          kind="lines"
          id="setting-admin"
          title="Admin addresses"
          description="Addresses allowed to open this page, besides the machine Antennarr runs on. One per line: an IP or a range like 100.64.0.0/10. Viewers are never admins; take care not to lock yourself out."
          initial={settings.admin.join('\n')}
          placeholder="100.64.0.0/10"
          disabled={!editable}
          onSave={(text) => update.mutateAsync({ admin: parseAddresses(text) })}
        />
        <PlexRows />
        <Row title="Who can use your tuners" description="Only the addresses of the viewers you add. With no viewers, anyone who can reach a port can.">
          <span className="text-sm font-medium">{settings.mode === 'open' ? 'Open to anyone' : 'Viewers only'}</span>
        </Row>
      </Section>

      <Section title="Updates">
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

      <Section title="Network" description="Antennarr listens on a port for each tuner. How viewers reach it (LAN, VPN, Tailscale) is up to you; built-in Tailscale is planned.">
        {devices.map((d) => (
          <Row key={d.name} title={deviceTitle(d)} description={several ? undefined : 'The port media servers connect to.'}>
            <span className="font-mono text-xs text-muted-foreground">{d.listen}</span>
          </Row>
        ))}
      </Section>
    </div>
  )
}
