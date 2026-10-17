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
  viewer: string
  client: string
  path: string
  channel?: string
  started: string
}

export interface Viewer {
  name: string
  addresses: string[]
  limit: number
  inUse: number
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

export interface AppState {
  time: string
  tuner: TunerState
  streams: Stream[]
  viewers: Viewer[]
  settings: Settings
  summary: Summary
  editable: boolean
}

export interface ViewerInput {
  name: string
  addresses: string[]
  limit: number
}

export interface SettingsPatch {
  sharedCap?: number
  ownerReserve?: number
  admin?: string[]
}
