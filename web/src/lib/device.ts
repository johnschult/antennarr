import type { DeviceState } from './types'

/** A tuner as far as naming goes: its key, the name the owner chose, and the model it reports. */
type Named = Pick<DeviceState, 'name' | 'label'> & { tuner?: Pick<DeviceState['tuner'], 'friendlyName'> }

/**
 * What to call a tuner on screen: the name the owner chose, else the model it reports
 * (HDHomeRun FLEX 4K), else its key. The name the owner chose is only for this page;
 * media servers are always told the device (HDHomeRun FLEX 4K).
 */
export const deviceTitle = (d: Named): string => d.label || d.tuner?.friendlyName || d.name

/** The model, to show beside a name the owner chose. Nothing when the title already is the model, so it isn't said twice. */
export const deviceModel = (d: Named): string | undefined => (d.label && d.tuner?.friendlyName && d.tuner.friendlyName !== d.label ? d.tuner.friendlyName : undefined)
