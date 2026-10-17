import type { AppState, Slot, Stream } from './types'

export type Owner =
  | { kind: 'viewer'; viewer: string }
  | { kind: 'remote' } // through Antennarr, but we can't tell who
  | { kind: 'local' } // the owner's own devices
  | { kind: 'reserved' }
  | { kind: 'free' }

export interface Tile {
  index: number
  slot: Slot | null // null if the tuner hasn't reported this tuner
  owner: Owner
  stream?: Stream
}

/**
 * Lay the tuner's slots out as tiles and say who holds each.
 *
 * The tuner only says "something is streaming to Antennarr's address"; the
 * stream list says which viewer. They are paired by channel number. Idle
 * tuners are shown as reserved up to the owner reserve, then free.
 */
export function buildTiles(state: Pick<AppState, 'tuner' | 'streams' | 'settings'>): Tile[] {
  const { tuner, streams, settings } = state
  const total = Math.max(tuner.tunerCount, tuner.slots.length)
  const unmatched = [...streams]
  const tiles: Tile[] = []

  for (let i = 0; i < total; i++) {
    const slot = tuner.slots[i] ?? null
    if (slot?.inUse && slot.viaAntennarr) {
      const at = unmatched.findIndex((s) => s.channel && s.channel === slot.channel)
      const stream = at >= 0 ? unmatched.splice(at, 1)[0] : undefined
      tiles.push({ index: i, slot, stream, owner: stream ? { kind: 'viewer', viewer: stream.viewer } : { kind: 'remote' } })
    } else if (slot?.inUse) {
      tiles.push({ index: i, slot, owner: { kind: 'local' } })
    } else {
      tiles.push({ index: i, slot, owner: { kind: 'free' } })
    }
  }

  let toReserve = settings.ownerReserve
  for (const t of tiles) {
    if (toReserve > 0 && t.owner.kind === 'free') {
      t.owner = { kind: 'reserved' }
      toReserve--
    }
  }
  return tiles
}

export function ownerLabel(o: Owner): string {
  switch (o.kind) {
    case 'viewer':
      return o.viewer
    case 'remote':
      return 'Remote'
    case 'local':
      return 'Local'
    case 'reserved':
      return 'Reserved'
    case 'free':
      return 'Free'
  }
}
