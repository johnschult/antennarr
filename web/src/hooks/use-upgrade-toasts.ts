import { useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'
import { firmwareDate } from '@/lib/format'
import type { UpgradePhase } from '@/lib/types'

const ACTIVE: UpgradePhase[] = ['downloading', 'sending', 'restarting', 'verifying']

/**
 * One toast when a firmware install ends, however it ended, wherever the person is on the page: it runs on
 * the server and takes minutes, so they may well have gone elsewhere. It only speaks for an install it saw
 * running: one that had already ended when the page opened is on the Tuners page, not announced again.
 */
export function useUpgradeToasts() {
  const { state } = useApp()
  // For each tuner, the install last seen (by its start time) and the phase it was in.
  const seen = useRef(new Map<string, { started: string; active: boolean }>())

  useEffect(() => {
    const names = new Set(state.devices.map((d) => d.name))
    for (const name of seen.current.keys()) if (!names.has(name)) seen.current.delete(name)

    for (const d of state.devices) {
      const u = d.upgrade
      if (!u) {
        seen.current.delete(d.name)
        continue
      }
      const active = ACTIVE.includes(u.phase)
      const before = seen.current.get(d.name)
      // Only an install that was running when it was last seen (and is the same one) can end in front of us.
      if (before && before.started === u.started && before.active && !active) {
        if (u.phase === 'done') toast.success(`Updated ${deviceTitle(d)} to ${firmwareDate(u.release)}`)
        else toast.error(`Firmware update failed: ${u.error ?? 'it did not finish'}`)
      }
      seen.current.set(d.name, { started: u.started, active })
    }
  }, [state.devices])
}
