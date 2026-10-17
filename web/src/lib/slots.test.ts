import { describe, expect, it } from 'vitest'
import { makeDevice, idleSlot, localSlot, remoteSlot } from '@/test/fixtures'
import { buildTiles, limitFor, ownerLabel } from './slots'

const NOW = '2026-10-17T10:05:00Z'
const kinds = (device = makeDevice()) => buildTiles(device).map((t) => t.owner.kind)

describe('buildTiles', () => {
  it('names the viewer by pairing a stream with its tuner by channel', () => {
    const tiles = buildTiles(makeDevice())
    expect(tiles.map((t) => t.owner)).toEqual([
      { kind: 'local' },
      { kind: 'viewer', viewer: 'John' },
      { kind: 'reserved' },
      { kind: 'free' },
    ])
    expect(tiles[1].stream?.id).toBe(1)
  })

  it('says "remote" when a stream through us has no matching stream record yet', () => {
    const s = makeDevice({ streams: [] })
    expect(kinds(s)[1]).toBe('remote')
  })

  it('pairs two streams on the same channel with two tuners, once each', () => {
    const s = makeDevice()
    s.tuner.slots = [remoteSlot(0, '5.1'), remoteSlot(1, '5.1'), idleSlot(2), idleSlot(3)]
    s.streams = [
      { id: 1, device: 'antenna', viewer: 'John', client: 'a', path: '/auto/v5.1', channel: '5.1', started: NOW },
      { id: 2, device: 'antenna', viewer: 'Mom', client: 'b', path: '/auto/v5.1', channel: '5.1', started: NOW },
    ]
    expect(buildTiles(s).slice(0, 2).map((t) => ownerLabel(t.owner))).toEqual(['John', 'Mom'])
  })

  it('does not match a stream tuned by frequency (no channel) to anyone', () => {
    const s = makeDevice()
    s.streams = [{ id: 1, device: 'antenna', viewer: 'John', client: 'a', path: '/auto/ch593000000-3', started: NOW }]
    expect(kinds(s)[1]).toBe('remote')
  })

  it('reserves idle tuners up to the owner reserve, then calls the rest free', () => {
    const s = makeDevice()
    s.tuner.slots = [idleSlot(0), idleSlot(1), idleSlot(2), idleSlot(3)]
    s.ownerReserve = 2
    expect(kinds(s)).toEqual(['reserved', 'reserved', 'free', 'free'])
    s.ownerReserve = 0
    expect(kinds(s)).toEqual(['free', 'free', 'free', 'free'])
  })

  it('never reserves a tuner that is in use', () => {
    const s = makeDevice()
    s.tuner.slots = [localSlot(0), localSlot(1), idleSlot(2), idleSlot(3)]
    s.ownerReserve = 3
    expect(kinds(s)).toEqual(['local', 'local', 'reserved', 'reserved'])
  })

  it('shows every tuner even before the tuner has reported its slots', () => {
    const s = makeDevice()
    s.tuner.slots = []
    s.ownerReserve = 0
    const tiles = buildTiles(s)
    expect(tiles).toHaveLength(4)
    expect(tiles.every((t) => t.slot === null && t.owner.kind === 'free')).toBe(true)
  })
})

describe('ownerLabel', () => {
  it('is readable for every kind', () => {
    expect(ownerLabel({ kind: 'viewer', viewer: 'John' })).toBe('John')
    expect(ownerLabel({ kind: 'remote' })).toBe('Remote')
    expect(ownerLabel({ kind: 'local' })).toBe('Local')
    expect(ownerLabel({ kind: 'reserved' })).toBe('Reserved')
    expect(ownerLabel({ kind: 'free' })).toBe('Free')
  })
})

describe('limitFor', () => {
  it("uses the device's own limit when there is one, else the default", () => {
    const v = { limit: 1, limits: { cable: 3, paused: 0 } }
    expect(limitFor(v, 'cable')).toBe(3)
    expect(limitFor(v, 'antenna')).toBe(1)
    expect(limitFor(v, 'paused')).toBe(0) // zero is a real override, not "unset"
    expect(limitFor({ limit: 2 }, 'anything')).toBe(2)
  })
})
