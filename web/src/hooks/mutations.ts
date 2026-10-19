import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { addChannel, addDevice, addViewer, checkFirmware, deleteChannel, deleteDevice, deleteViewer, dismissRestart, dismissUpgrade, getState, installFirmware, patchDevice, patchSettings, replaceChannel, restartTuner, replaceViewer, testChannel } from '@/lib/api'
import { deviceTitle } from '@/lib/device'
import type { AppState, ChannelInput, DeviceInput, UpgradeRequest, DevicePatch, SettingsPatch, ViewerInput } from '@/lib/types'
import { STATE_KEY } from './use-live-state'

/** What a firmware check turned up across the tuners: newer firmware, none, or no clear answer. */
function firmwareFound(state: AppState | undefined): 'update' | 'none' | 'unclear' {
  const states = state?.devices.map((d) => d.firmware.state) ?? []
  if (states.some((s) => s === 'update-available' || s === 'possible-update')) return 'update'
  return states.length > 0 && states.every((s) => s === 'up-to-date') ? 'none' : 'unclear'
}

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
    onSuccess: async () => {
      // The check has finished by the time the server answers; wait for the new status so the
      // toast can say what it found, and the button stays busy until then.
      const found = firmwareFound(await qc.fetchQuery({ queryKey: STATE_KEY, queryFn: getState, staleTime: 0 }))
      if (found === 'update') toast.success('New firmware found')
      else if (found === 'none') toast.success('No new firmware')
      else toast.info("Couldn't tell whether there is new firmware")
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
  const channelAdd = useMutation({
    mutationFn: (c: ChannelInput) => addChannel(c),
    onSuccess: (c) => {
      toast.success(`Added ${c.name}`)
      void refresh()
    },
  })
  const channelReplace = useMutation({
    mutationFn: ({ id, input }: { id: string; input: ChannelInput }) => replaceChannel(id, input),
    onSuccess: (c) => {
      toast.success(`Saved ${c.name}`)
      void refresh()
    },
  })
  const channelRemove = useMutation({
    mutationFn: ({ id }: { id: string; name: string }) => deleteChannel(id),
    onSuccess: (_, { name }) => {
      toast.success(`Removed ${name}`)
      void refresh()
    },
  })
  // Like the firmware check: the button shows it is working, and the one toast says how it went.
  const channelTest = useMutation({
    mutationFn: ({ request }: { name: string; request: Parameters<typeof testChannel>[0] }) => testChannel(request),
    onSuccess: (_, { name }) => toast.success(`Test sent to ${name}`),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'The test did not go through'),
  })
  // Installing runs on the server, in the background. Starting it is a request that answers at once; how it
  // goes arrives in the state, and the toast for how it ended is in useUpgradeToasts, so it shows wherever the
  // person is on the page. A request that is refused says why.
  const install = useMutation({
    mutationFn: ({ device, request }: { device: string; request: UpgradeRequest }) => installFirmware(device, request),
    onSuccess: () => void refresh(),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Could not start the update'),
  })
  const dismiss = useMutation({
    mutationFn: ({ device }: { device: string }) => dismissUpgrade(device),
    onSuccess: () => void refresh(),
  })
  const restart = useMutation({
    mutationFn: ({ device, interrupt }: { device: string; interrupt: boolean }) => restartTuner(device, interrupt),
    onSuccess: () => void refresh(),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Could not restart the tuner'),
  })
  const dismissRestartResult = useMutation({
    mutationFn: ({ device }: { device: string }) => dismissRestart(device),
    onSuccess: () => void refresh(),
  })
  return { add, replace, remove, settings, device, firmware, install, dismiss, restart, dismissRestartResult, addTuner, removeTuner, channelAdd, channelReplace, channelRemove, channelTest }
}
