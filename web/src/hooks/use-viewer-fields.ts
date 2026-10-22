import { useState } from 'react'
import { deviceTitle } from '@/lib/device'
import { parseAddresses } from '@/lib/format'
import type { DeviceState, Viewer, ViewerInput } from '@/lib/types'

export const isCount = (s: string) => s.trim() !== '' && Number.isInteger(Number(s)) && Number(s) >= 0

/** The viewer form's values, and what to send when they are good. Shared by editing a viewer and the add-viewer wizard. */
export function useViewerFields(viewer: Viewer | undefined, devices: DeviceState[]) {
  const [name, setName] = useState(viewer?.name ?? '')
  const [addresses, setAddresses] = useState(viewer?.addresses.join('\n') ?? '')
  const [limit, setLimit] = useState(String(viewer?.limit ?? 1))
  // A blank per-device box means "use the default".
  const [overrides, setOverrides] = useState<Record<string, string>>(
    Object.fromEntries(Object.entries(viewer?.limits ?? {}).map(([k, v]) => [k, String(v)])),
  )

  /** The input to send, or why it can't be. */
  function build(): { input: ViewerInput } | { error: string } {
    const input: ViewerInput = { name: name.trim(), addresses: parseAddresses(addresses), limit: Number(limit) }
    if (!input.name) return { error: 'Give the viewer a name.' }
    if (input.addresses.length === 0) return { error: 'Add at least one address.' }
    if (!isCount(limit)) return { error: 'The tuner limit must be 0 or more.' }
    const limits: Record<string, number> = {}
    for (const d of devices) {
      const raw = (overrides[d.name] ?? '').trim()
      if (raw === '') continue
      if (!isCount(raw)) return { error: `The limit for ${deviceTitle(d)} must be 0 or more.` }
      limits[d.name] = Number(raw)
    }
    if (devices.length > 1) input.limits = limits // an empty object clears any earlier overrides
    return { input }
  }

  return { name, setName, addresses, setAddresses, limit, setLimit, overrides, setOverrides, build }
}

export type ViewerFieldsState = ReturnType<typeof useViewerFields>

