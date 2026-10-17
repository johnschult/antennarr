import type { AppState, AuthStatus, DevicePatch, DeviceState, PlexPollStatus, Settings, SettingsPatch, Viewer, ViewerInput } from './types'

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
export const deleteViewer = (name: string, allowOpen = false) =>
  request<void>('DELETE', `/viewers/${encodeURIComponent(name)}${allowOpen ? '?allowOpen=true' : ''}`)
export const patchSettings = (p: SettingsPatch) => request<Settings>('PATCH', '/settings', p)
export const patchDevice = (name: string, p: DevicePatch) =>
  request<DeviceState>('PATCH', `/devices/${encodeURIComponent(name)}`, p)

/** Read SiliconDust's changelog now. The new status arrives over the event stream. */
export const checkFirmware = () => request<{ checkedAt: string }>('POST', '/firmware/check')

export const removePlexAdmin = (id: number) => request<void>('DELETE', `/plex/admins/${id}`)

export const getAuthStatus = () => request<AuthStatus>('GET', '/status', undefined, AUTH)
export const startPlex = (add = false) =>
  request<{ pinId: number; authUrl: string }>('POST', `/plex${add ? '/add' : ''}/start`, {}, AUTH)
export const pollPlex = (pinId: number, add = false) =>
  request<PlexPollStatus>('POST', `/plex${add ? '/add' : ''}/poll`, { pinId }, AUTH)
export const signOut = () => request<unknown>('POST', '/logout', undefined, AUTH)
