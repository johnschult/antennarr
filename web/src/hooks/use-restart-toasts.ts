import { useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { useApp } from '@/hooks/app-context'
import { deviceTitle } from '@/lib/device'

/**
 * One toast when a restart someone asked for ends, however it ended, wherever they are on the page. It only
 * speaks for a restart it saw running: one that had already ended when the page opened is not announced.
 */
export function useRestartToasts() {
  const { state } = useApp()
  const seen = useRef(new Map<string, { started: string; active: boolean }>())

  useEffect(() => {
    const names = new Set(state.devices.map((d) => d.name))
    for (const name of seen.current.keys()) if (!names.has(name)) seen.current.delete(name)

    for (const d of state.devices) {
      const r = d.restart
      if (!r) {
        seen.current.delete(d.name)
        continue
      }
      const active = r.phase === 'restarting'
      const before = seen.current.get(d.name)
      if (before && before.started === r.started && before.active && !active) {
        if (r.phase === 'done') toast.success(`${deviceTitle(d)} restarted`)
        else toast.error(`Restart failed: ${r.error ?? 'it did not finish'}`)
      }
      seen.current.set(d.name, { started: r.started, active })
    }
  }, [state.devices])
}
