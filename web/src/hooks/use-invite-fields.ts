import { useState } from 'react'
import { deviceTitle } from '@/lib/device'
import type { DeviceState, InviteInput } from '@/lib/types'
import { isCount } from './use-viewer-fields'

/** The invite form's values, and what to send when they are good. The same name and limits as a viewer, without addresses. */
export function useInviteFields(devices: DeviceState[]) {
  const [name, setName] = useState('')
  const [limit, setLimit] = useState('1')
  const [overrides, setOverrides] = useState<Record<string, string>>({})

  /** The input to send, or why it can't be. */
  function build(): { input: InviteInput } | { error: string } {
    const input: InviteInput = { name: name.trim(), limit: Number(limit) }
    if (!input.name) return { error: 'Give the viewer a name.' }
    if (!isCount(limit)) return { error: 'The tuner limit must be 0 or more.' }
    const limits: Record<string, number> = {}
    for (const d of devices) {
      const raw = (overrides[d.name] ?? '').trim()
      if (raw === '') continue
      if (!isCount(raw)) return { error: `The limit for ${deviceTitle(d)} must be 0 or more.` }
      limits[d.name] = Number(raw)
    }
    if (Object.keys(limits).length > 0) input.limits = limits
    return { input }
  }

  return { name, setName, limit, setLimit, overrides, setOverrides, build }
}
