import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { addDevice, addViewer, checkFirmware, deleteDevice, deleteViewer, patchDevice, patchSettings, replaceViewer } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import type { DeviceInput, DevicePatch, SettingsPatch, ViewerInput } from '@/lib/types'
import { STATE_KEY } from './use-live-state'

/**
 * Edits go to the server, which saves them and pushes the new state over the
 * event stream; invalidating as well covers the stream being down. Errors are
 * left to the caller so a form can show them in place.
 */
export function useEdits() {
  const qc = useQueryClient()
  const refresh = () => qc.invalidateQueries({ queryKey: STATE_KEY })

  const add = useMutation({
    mutationFn: (v: ViewerInput) => addViewer(v),
    onSuccess: (_, v) => {
      toast.success(`Added ${v.name}`)
      void refresh()
    },
  })
  const replace = useMutation({
    mutationFn: ({ name, input }: { name: string; input: ViewerInput }) => replaceViewer(name, input),
    onSuccess: (_, { input }) => {
      toast.success(`Saved ${input.name}`)
      void refresh()
    },
  })
  const remove = useMutation({
    mutationFn: ({ name }: { name: string }) => deleteViewer(name),
    onSuccess: (_, { name }) => {
      toast.success(`Removed ${name}`)
      void refresh()
    },
  })
  const settings = useMutation({
    mutationFn: (p: SettingsPatch) => patchSettings(p),
    onSuccess: () => {
      toast.success('Settings saved')
      void refresh()
    },
  })
  const device = useMutation({
    mutationFn: ({ name, patch }: { name: string; patch: DevicePatch }) => patchDevice(name, patch),
    onSuccess: () => {
      toast.success('Settings saved')
      void refresh()
    },
  })
  const firmware = useMutation({
    mutationFn: () => checkFirmware(),
    onSuccess: () => {
      toast.success("Read SiliconDust's changelog")
      void refresh()
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Could not check for firmware'),
  })
  const addTuner = useMutation({
    mutationFn: (v: DeviceInput) => addDevice(v),
    onSuccess: (d) => {
      toast.success(`Added ${deviceTitle(d)}`)
      void refresh()
    },
  })
  const removeTuner = useMutation({
    mutationFn: ({ name }: { name: string }) => deleteDevice(name),
    onSuccess: (_, { name }) => {
      toast.success(`Removed ${name}`)
      void refresh()
    },
  })
  return { add, replace, remove, settings, device, firmware, addTuner, removeTuner }
}
