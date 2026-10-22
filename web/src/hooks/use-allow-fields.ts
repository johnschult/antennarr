import { useState } from 'react'
import { deviceTitle } from '@/lib/device'
import type { AccessRequest, AllowInput, DeviceState } from '@/lib/types'
import { isCount } from './use-viewer-fields'

/** A name to offer for someone who asked: the start of their login or device name, or nothing for a bare address. */
export function suggestName(r: AccessRequest): string {
  const base = r.login ? r.login.split('@')[0] : r.node ? r.node.split('.')[0] : ''
  return base.replace(/[._-]+/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()).trim()
}

/** The allow form's values, and what to send when they are good: a name and the tuner limits. */
export function useAllowFields(request: AccessRequest, devices: DeviceState[]) {
  const [name, setName] = useState(suggestName(request))
  const [limit, setLimit] = useState('1')
  const [overrides, setOverrides] = useState<Record<string, string>>({})

  function build(): { input: AllowInput } | { error: string } {
    const input: AllowInput = { name: name.trim(), limit: Number(limit) }
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
