// Shapes of the Go API (internal/api). Keep in step with api.go.

export interface Slot {
  resource: string
  inUse: boolean
  channel?: string
  channelName?: string
  targetIP?: string
  viaAntennarr: boolean
  signalStrength?: number
  signalQuality?: number
  bitsPerSecond?: number
}

export type EventKind = 'ok' | 'info' | 'warn' | 'err'

export interface TunerEvent {
  time: string
  kind: EventKind
  text: string
}

export interface TunerState {
  name?: string
  online: boolean
  id: string
  friendlyName?: string
  model?: string
  baseURL: string
  tunerCount: number
  slots: Slot[]
  lastSeen: string
  since: string
  events: TunerEvent[]
}

export interface Stream {
  id: number
  device: string
  viewer: string
  client: string
  path: string
  channel?: string
  started: string
}

export interface Viewer {
  name: string
  addresses: string[]
  /** Tuners at once on a device, unless `limits` says otherwise for it. */
  limit: number
  /** Per-device overrides, by device name. */
  limits?: Record<string, number>
  inUse: number
  inUseByDevice: Record<string, number>
  lastSeen: string | null
}

export interface Settings {
  mode: 'open' | 'enforced'
  sharedCap: number
  ownerReserve: number
  admin: string[]
}

export interface Summary {
  total: number
  inUse: number
  remote: number
  local: number
  reserved: number
  free: number
}

/** One HDHomeRun that Antennarr fronts. */
export interface DeviceState {
  name: string
  listen: string
  tuner: TunerState
  streams: Stream[]
  summary: Summary
  /** The values in force for this device (its own, or the defaults). */
  ownerReserve: number
  sharedCap: number
}

export interface AppState {
  time: string
  devices: DeviceState[]
  viewers: Viewer[]
  settings: Settings
  editable: boolean
}

export interface ViewerInput {
  name: string
  addresses: string[]
  limit: number
  limits?: Record<string, number>
}

export interface SettingsPatch {
  sharedCap?: number
  ownerReserve?: number
  admin?: string[]
}

export interface DevicePatch {
  sharedCap?: number
  ownerReserve?: number
}
