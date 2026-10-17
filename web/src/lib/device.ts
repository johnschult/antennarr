import type { DeviceState } from './types'

/** What to call a tuner on screen: the name the owner chose, else its key. */
export const deviceTitle = (d: Pick<DeviceState, 'name' | 'label'>): string => d.label || d.name
