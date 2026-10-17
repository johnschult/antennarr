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

/** What the tuner itself says about its firmware. */
export interface TunerFirmware {
  name?: string
  version?: string
  upgradeAvailable?: string
}

export interface FirmwareNote {
  scope?: string
  text: string
}

export interface FirmwareUpdate {
  version: string
  /** True when a note is plainly for this model; false when it only might be. */
  certain: boolean
  notes: FirmwareNote[]
}

export type FirmwareState = 'up-to-date' | 'update-available' | 'possible-update' | 'unknown' | 'disabled'

/** Where a tuner's firmware stands against SiliconDust's changelog. */
export interface FirmwareStatus {
  state: FirmwareState
  current: string
  latest: string
  deviceReported?: string
  updates: FirmwareUpdate[]
  modelRecognised: boolean
  /** Zero time ("0001-01-01...") when it has never been read. */
  checkedAt: string
  error?: string
}

export interface TunerState {
  name?: string
  online: boolean
  id: string
  friendlyName?: string
  model?: string
  baseURL: string
  tunerCount: number
  firmware: TunerFirmware
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
  /** Whether SiliconDust's changelog is read (one request every 12 hours). */
  firmwareCheck: boolean
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
  /** The stable key: used in settings, limits and URLs. */
  name: string
  /** The display name the owner chose, if any. */
  label?: string
  listen: string
  tuner: TunerState
  streams: Stream[]
  summary: Summary
  /** The values in force for this device (its own, or the defaults). */
  ownerReserve: number
  sharedCap: number
  firmware: FirmwareStatus
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
  firmwareCheck?: boolean
}

export interface DevicePatch {
  sharedCap?: number
  ownerReserve?: number
  /** The display name; an empty string clears it. */
  label?: string
}
