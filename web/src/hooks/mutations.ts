import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { addViewer, deleteViewer, patchSettings, replaceViewer } from '@/lib/api'
import type { SettingsPatch, ViewerInput } from '@/lib/types'
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
    mutationFn: ({ name, allowOpen }: { name: string; allowOpen: boolean }) => deleteViewer(name, allowOpen),
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
  return { add, replace, remove, settings }
}
