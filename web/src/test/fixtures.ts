import type { AppState, Slot, Viewer } from '@/lib/types'

export const idleSlot = (n: number): Slot => ({ resource: `tuner${n}`, inUse: false, viaAntennarr: false })

export const localSlot = (n: number, channel = '2.1', name = 'WBAY'): Slot => ({
  resource: `tuner${n}`, inUse: true, channel, channelName: name, targetIP: '192.168.1.77', viaAntennarr: false, bitsPerSecond: 12_800_000,
})

export const remoteSlot = (n: number, channel = '5.1', name = 'WFRV'): Slot => ({
  resource: `tuner${n}`, inUse: true, channel, channelName: name, targetIP: '192.168.1.20', viaAntennarr: true, bitsPerSecond: 14_200_000,
})

export const john: Viewer = { name: 'John', addresses: ['100.67.117.32'], limit: 2, inUse: 1, lastSeen: '2026-10-17T10:00:00Z' }
export const mom: Viewer = { name: 'Mom', addresses: ['10.8.0.0/24'], limit: 1, inUse: 0, lastSeen: null }

/** A realistic state: 4 tuners, one local, one of John's, one reserved, one free. */
export function makeState(over: Partial<AppState> = {}): AppState {
  const base: AppState = {
    time: '2026-10-17T10:05:00Z',
    tuner: {
      online: true, id: '10ABCC87', friendlyName: 'Steve FLEX 4K', model: 'HDFX-4K', baseURL: 'http://192.168.1.132',
      tunerCount: 4, slots: [localSlot(0), remoteSlot(1), idleSlot(2), idleSlot(3)],
      lastSeen: '2026-10-17T10:04:58Z', since: '2026-10-17T08:00:00Z',
      events: [
        { time: '2026-10-17T08:00:00Z', kind: 'ok', text: 'Tuner online at http://192.168.1.132' },
        { time: '2026-10-17T09:30:00Z', kind: 'warn', text: 'Mom was refused a tuner: all 1 of your tuners are in use' },
      ],
    },
    streams: [{ id: 1, viewer: 'John', client: '100.67.117.32:4000', path: '/auto/v5.1', channel: '5.1', started: '2026-10-17T10:00:00Z' }],
    viewers: [john, mom],
    settings: { mode: 'enforced', sharedCap: 0, ownerReserve: 1, admin: [] },
    summary: { total: 4, inUse: 2, remote: 1, local: 1, reserved: 1, free: 2 },
    editable: true,
  }
  return { ...base, ...over }
}
