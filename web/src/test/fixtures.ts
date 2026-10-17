import type { AppState, DeviceState, FirmwareStatus, Slot, Viewer } from '@/lib/types'

export const idleSlot = (n: number): Slot => ({ resource: `tuner${n}`, inUse: false, viaAntennarr: false })

export const localSlot = (n: number, channel = '2.1', name = 'WBAY'): Slot => ({
  resource: `tuner${n}`, inUse: true, channel, channelName: name, targetIP: '192.168.1.77', viaAntennarr: false, bitsPerSecond: 12_800_000,
})

export const remoteSlot = (n: number, channel = '5.1', name = 'WFRV'): Slot => ({
  resource: `tuner${n}`, inUse: true, channel, channelName: name, targetIP: '192.168.1.20', viaAntennarr: true, bitsPerSecond: 14_200_000,
})

export const john: Viewer = {
  name: 'John', addresses: ['100.67.117.32'], limit: 2, inUse: 1, inUseByDevice: { antenna: 1 }, lastSeen: '2026-10-17T10:00:00Z',
}
export const mom: Viewer = { name: 'Mom', addresses: ['10.8.0.0/24'], limit: 1, inUse: 0, inUseByDevice: {}, lastSeen: null }

export const upToDate: FirmwareStatus = {
  state: 'up-to-date', current: '20260326', latest: '20260326', updates: [], modelRecognised: true, checkedAt: '2026-10-17T09:00:00Z',
}

export const updateAvailable: FirmwareStatus = {
  state: 'update-available', current: '20250815', latest: '20260326', modelRecognised: true, checkedAt: '2026-10-17T09:00:00Z',
  updates: [
    { version: '20260326', certain: true, notes: [{ scope: 'ATSC3 models', text: 'Security update.' }] },
    { version: '20260313', certain: true, notes: [{ scope: 'All models', text: 'Upgrade TLS root certificates.' }, { text: 'A line with no scope.' }] },
  ],
}

/** The antenna box: 4 tuners, one local, one of John's, one reserved, one free. */
export function makeDevice(over: Partial<DeviceState> = {}): DeviceState {
  return {
    name: 'antenna',
    listen: ':5004',
    tuner: {
      name: 'antenna', online: true, id: '10ABCC87', friendlyName: 'Steve FLEX 4K', model: 'HDFX-4K', baseURL: 'http://192.168.1.132',
      tunerCount: 4, firmware: { name: 'hdhomerun_dvr_atsc3', version: '20260326' }, slots: [localSlot(0), remoteSlot(1), idleSlot(2), idleSlot(3)],
      lastSeen: '2026-10-17T10:04:58Z', since: '2026-10-17T08:00:00Z',
      events: [
        { time: '2026-10-17T08:00:00Z', kind: 'ok', text: 'Tuner online at http://192.168.1.132' },
        { time: '2026-10-17T09:30:00Z', kind: 'warn', text: 'Mom was refused a tuner: all 1 of your tuners are in use' },
      ],
    },
    streams: [{ id: 1, device: 'antenna', viewer: 'John', client: '100.67.117.32:4000', path: '/auto/v5.1', channel: '5.1', started: '2026-10-17T10:00:00Z' }],
    summary: { total: 4, inUse: 2, remote: 1, local: 1, reserved: 1, free: 2 },
    ownerReserve: 1,
    sharedCap: 0,
    firmware: upToDate,
    ...over,
  }
}

/** The cable box: 2 tuners, both idle, no reserve. */
export function makeCableDevice(over: Partial<DeviceState> = {}): DeviceState {
  return makeDevice({
    name: 'cable',
    listen: ':5005',
    tuner: {
      name: 'cable', online: true, id: '1111AAA2', friendlyName: 'Cable HDHR', model: 'HDHR5-4DT', baseURL: 'http://192.168.1.133',
      tunerCount: 2, firmware: { name: 'hdhomerun5_dvbt', version: '20260326' }, slots: [idleSlot(0), idleSlot(1)], lastSeen: '2026-10-17T10:04:58Z', since: '2026-10-17T08:00:00Z',
      events: [{ time: '2026-10-17T08:05:00Z', kind: 'ok', text: 'Tuner online at http://192.168.1.133' }],
    },
    streams: [],
    summary: { total: 2, inUse: 0, remote: 0, local: 0, reserved: 0, free: 2 },
    ownerReserve: 0,
    ...over,
  })
}

export function makeState(over: Partial<AppState> = {}): AppState {
  return {
    time: '2026-10-17T10:05:00Z',
    devices: [makeDevice()],
    viewers: [john, mom],
    settings: { mode: 'enforced', sharedCap: 0, ownerReserve: 1, admin: [], firmwareCheck: true },
    editable: true,
    ...over,
  }
}

/** Two devices; John has a lower limit on the antenna box than on the cable box. */
export function makeTwoDeviceState(): AppState {
  return makeState({
    devices: [makeDevice(), makeCableDevice()],
    viewers: [{ ...john, limits: { cable: 2 } }, mom],
  })
}
