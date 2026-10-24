import type { AppState, DeviceState, ProblemKind, TunerProblem } from '@/lib/types'

/** How long a refusal stays on the page after it last happened. An empty channel list stays until the tuner has channels again. */
const RECENT_MS = 15 * 60 * 1000

export interface ShownProblem {
  device: DeviceState
  p: TunerProblem
}

/** Whether a problem is still worth showing at `now`. */
export function isRecent(p: TunerProblem, now: number): boolean {
  const empty: ProblemKind = 'no-channels'
  return p.kind === empty || now - new Date(p.last).getTime() < RECENT_MS
}

/** The problems worth showing now, across every tuner. */
export function recentProblems(state: AppState): ShownProblem[] {
  const now = new Date(state.time).getTime()
  return state.devices.flatMap((device) => (device.tuner.problems ?? []).filter((p) => isRecent(p, now)).map((p) => ({ device, p })))
}
