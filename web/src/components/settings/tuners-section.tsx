import { useState } from 'react'
import { AddTunerDialog } from '@/components/add-tuner-dialog'
import { RemoveTunerDialog } from '@/components/remove-tuner-dialog'
import { Row, Section } from '@/components/settings/section'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'
import type { DeviceState } from '@/lib/types'

/** The tuners being shared, with a way to add another or remove one. */
export function TunersSection() {
  const { state } = useApp()
  const { devices, editable } = state
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<DeviceState | null>(null)

  return (
    <Section title="Tuners" description="The HDHomeRun tuners Antennarr shares. Each one is reached on its own port.">
      {devices.map((d) => (
        <Row
          key={d.name}
          title={deviceTitle(d)}
          description={`${d.tuner.friendlyName ?? 'HDHomeRun'}${d.tuner.id ? ` · ${d.tuner.id}` : ''} · port ${d.listen.replace(/^.*:/, '')}`}
        >
          <Button variant="outline" size="sm" disabled={!editable} aria-label={`Remove ${deviceTitle(d)}`} onClick={() => setRemoving(d)}>
            Remove
          </Button>
        </Row>
      ))}
      <Row title="Add a tuner" description={editable ? 'Find one on your network, or give its address.' : 'Antennarr has no settings file to save tuners in.'}>
        <Button size="sm" disabled={!editable} onClick={() => setAdding(true)}>
          Add tuner
        </Button>
      </Row>
      <AddTunerDialog open={adding} onOpenChange={setAdding} />
      <RemoveTunerDialog device={removing} onClose={() => setRemoving(null)} />
    </Section>
  )
}
