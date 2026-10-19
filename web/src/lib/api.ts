import type { AppState, ChannelInput, DeviceInfo, DeviceInput, HistoryDoc, FirewallInfo, FoundTuner, NetworkAddress, AuthStatus, DevicePatch, DeviceState, PlexPollStatus, NotifyChannel, Settings, SettingsPatch, UpgradeRequest, Viewer, ViewerInput } from './types'

// The UI is served from /antennarr/ and the API lives beside it.
export const API = `${import.meta.env.BASE_URL}api`

/** An error reply from the Go API; message is safe to show to the user. */
export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Signing in lives beside the API, not inside it: it has to work before you are an admin.
export const AUTH = `${import.meta.env.BASE_URL}auth`

async function request<T>(method: string, path: string, body?: unknown, base: string = API): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${base}${path}`, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(0, 'Could not reach Antennarr')
  }
  if (res.status === 204) return undefined as T

  const text = await res.text()
  let data: unknown
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    data = undefined
  }
  if (!res.ok) {
    const msg = (data as { error?: string } | undefined)?.error
    throw new ApiError(res.status, msg ?? `Request failed (${res.status})`)
  }
  return data as T
}

export const getState = () => request<AppState>('GET', '/state')
export const addViewer = (v: ViewerInput) => request<Viewer>('POST', '/viewers', v)
export const replaceViewer = (name: string, v: ViewerInput) =>
  request<Viewer>('PUT', `/viewers/${encodeURIComponent(name)}`, v)
export const deleteViewer = (name: string) => request<void>('DELETE', `/viewers/${encodeURIComponent(name)}`)
export const patchSettings = (p: SettingsPatch) => request<Settings>('PATCH', '/settings', p)
export const patchDevice = (name: string, p: DevicePatch) =>
  request<DeviceState>('PATCH', `/devices/${encodeURIComponent(name)}`, p)

/** Search the network for tuners now. Takes a few seconds. */
export const discoverTuners = () => request<{ tuners: FoundTuner[] }>('POST', '/discover').then((r) => r.tuners)
export const addDevice = (d: DeviceInput) => request<DeviceState>('POST', '/devices', d)
export const deleteDevice = (name: string) => request<void>('DELETE', `/devices/${encodeURIComponent(name)}`)

/** Read SiliconDust's changelog now. The new status arrives over the event stream. */
export const checkFirmware = () => request<{ checkedAt: string }>('POST', '/firmware/check')

export const addChannel = (c: ChannelInput) => request<NotifyChannel>('POST', '/notifications/channels', c)
export const replaceChannel = (id: string, c: ChannelInput) => request<NotifyChannel>('PUT', `/notifications/channels/${encodeURIComponent(id)}`, c)
export const deleteChannel = (id: string) => request<void>('DELETE', `/notifications/channels/${encodeURIComponent(id)}`)
/** Send one test message to a channel: a saved one by `id`, or the form's values, or a saved one with edits not yet saved. */
export const testChannel = (t: { id?: string; channel?: ChannelInput }) => request<{ ok: true }>('POST', '/notifications/test', t)

/** Install the latest firmware on a tuner. It runs in the background: progress is in the state. */
export const installFirmware = (device: string, r: UpgradeRequest) =>
  request<{ release: string; from: string }>('POST', `/devices/${encodeURIComponent(device)}/firmware/upgrade`, r)
/** Clear a finished install's result. */
export const getDeviceInfo = (device: string, refresh = false) =>
  request<DeviceInfo>('GET', `/devices/${encodeURIComponent(device)}/info${refresh ? '?refresh=1' : ''}`)
export const restartTuner = (device: string, interrupt: boolean) =>
  request<void>('POST', `/devices/${encodeURIComponent(device)}/restart`, { interrupt })
export const dismissRestart = (device: string) => request<void>('DELETE', `/devices/${encodeURIComponent(device)}/restart`)
export const getHistory = (days: number) => request<HistoryDoc>('GET', `/history?days=${days}`)
export const dismissUpgrade = (device: string) => request<void>('DELETE', `/devices/${encodeURIComponent(device)}/firmware/upgrade`)

export const removePlexAdmin = (id: number) => request<void>('DELETE', `/plex/admins/${id}`)

export const getAuthStatus = () => request<AuthStatus>('GET', '/status', undefined, AUTH)
export const startPlex = (add = false) =>
  request<{ pinId: number; authUrl: string }>('POST', `/plex${add ? '/add' : ''}/start`, {}, AUTH)
export const pollPlex = (pinId: number, add = false) =>
  request<PlexPollStatus>('POST', `/plex${add ? '/add' : ''}/poll`, { pinId }, AUTH)
export const signOut = () => request<unknown>('POST', '/logout', undefined, AUTH)

/** Where a channel's logo is served from: through Antennarr, never straight from SiliconDust. */
export const channelIconUrl = (device: string, channel: string) =>
  `${API}/devices/${encodeURIComponent(device)}/guide/${encodeURIComponent(channel)}/icon`

/** Where a program's picture is served from: through Antennarr, never straight from SiliconDust. The token is the guide's `now.poster`. */
export const programPosterUrl = (device: string, channel: string, token: string) =>
  `${API}/devices/${encodeURIComponent(device)}/guide/${encodeURIComponent(channel)}/poster/${encodeURIComponent(token)}`

/** This machine's Tailscale and LAN addresses and its firewall, for the setup guide. */
export const getNetwork = () => request<{ addresses: NetworkAddress[]; firewall?: FirewallInfo }>('GET', '/network')
