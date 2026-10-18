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

/** A Plex account that may sign in to the admin pages. */
export interface PlexAccount {
  id: number
  name: string
}

export interface PlexSettings {
  enabled: boolean
  admins: PlexAccount[]
}

/** Who this browser is, as far as signing in goes. Public: it is how the page knows whether to offer sign-in. */
export interface AuthStatus {
  plexEnabled: boolean
  signedIn: { id: number; name: string } | null
  /** Whether this request may use the admin pages at all (by address, or by being signed in). */
  admin: boolean
}

export type PlexPollStatus =
  | { status: 'pending' }
  | { status: 'expired' }
  | { status: 'ok'; user?: { id: number; name: string }; account?: { id: number; name: string } }
  | { status: 'denied'; user: { name: string } }

export interface Settings {
  mode: 'no-viewers' | 'enforced'
  sharedCap: number
  ownerReserve: number
  admin: string[]
  /** Whether SiliconDust's changelog is read (one request every 12 hours). */
  firmwareCheck: boolean
  guide: boolean
  plex: PlexSettings
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
  /** What the programme guide says about each channel being watched, by channel number. */
  guide: Record<string, GuideEntry>
}

/** A channel as the programme guide knows it. */
export interface GuideEntry {
  name?: string
  /** Whether a logo can be fetched (see channelIconUrl). */
  icon: boolean
  /** What is on right now. */
  now?: { title: string; subTitle?: string; start: string; stop: string }
}

export interface AppState {
  time: string
  devices: DeviceState[]
  viewers: Viewer[]
  settings: Settings
  editable: boolean
  /** This build's version ("2026.10.0", or "dev") and its page on GitHub. */
  version: string
  versionUrl: string
  /** The documentation as it was at this version. */
  docsUrl: string
}

/** A tuner found on the network by a search. `added` is the device it already is, if any. */
export interface FoundTuner {
  id: string
  address: string
  tuners: number
  added: string
}

/** A tuner to start sharing. Only the address (or DeviceID) is needed; the rest is chosen. */
export interface DeviceInput {
  tuner: string
  id?: string
  name?: string
  listen?: string
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
  guide?: boolean
  plexEnabled?: boolean
}

export interface DevicePatch {
  sharedCap?: number
  ownerReserve?: number
  /** The display name; an empty string clears it. */
  label?: string
  /** Where the tuner is reached, e.g. ":5010". The server opens it before letting go of the old one. */
  listen?: string
}

/** One way to reach the machine running Antennarr, read from its network interfaces. */
export interface NetworkAddress {
  ip: string
  kind: 'tailscale' | 'lan'
  interface: string
}

/** Which firewall looks to be in use on the machine running Antennarr. Its rules can't be read, so this is only which tool is switched on. */
export interface FirewallInfo {
  tool: '' | 'ufw' | 'firewalld' | 'iptables'
  active: boolean
}
