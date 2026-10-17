import { describe, expect, it } from 'vitest'
import { duration, formatMbps, parseAddresses, timeAgo } from './format'

const now = new Date('2026-10-17T12:00:00Z')

describe('formatMbps', () => {
  it('shows one decimal under 10 and none above', () => {
    expect(formatMbps(14_200_000)).toBe('14 Mbps')
    expect(formatMbps(8_540_000)).toBe('8.5 Mbps')
    expect(formatMbps(0)).toBe('0 Mbps')
    expect(formatMbps(undefined)).toBe('0 Mbps')
  })
})

describe('timeAgo', () => {
  const ago = (s: number) => new Date(now.getTime() - s * 1000).toISOString()
  it('steps through units', () => {
    expect(timeAgo(ago(3), now)).toBe('just now')
    expect(timeAgo(ago(42), now)).toBe('42s ago')
    expect(timeAgo(ago(4 * 60 + 5), now)).toBe('4m ago')
    expect(timeAgo(ago(3 * 3600), now)).toBe('3h ago')
    expect(timeAgo(ago(2 * 86400), now)).toBe('2d ago')
  })
  it('handles missing values and future times', () => {
    expect(timeAgo(null, now)).toBe('never')
    expect(timeAgo(undefined, now)).toBe('never')
    expect(timeAgo(new Date(now.getTime() + 60_000).toISOString(), now)).toBe('just now')
  })
})

describe('duration', () => {
  it('shows minutes and seconds, then hours', () => {
    expect(duration('2026-10-17T11:52:57Z', now)).toBe('7m 03s')
    expect(duration('2026-10-17T10:55:00Z', now)).toBe('1h 05m')
    expect(duration('2026-10-17T12:00:00Z', now)).toBe('0m 00s')
  })
})

describe('parseAddresses', () => {
  it('splits on lines, commas and spaces and drops blanks', () => {
    expect(parseAddresses('100.67.117.32\n10.8.0.0/24, 10.9.0.6  \n\n')).toEqual(['100.67.117.32', '10.8.0.0/24', '10.9.0.6'])
    expect(parseAddresses('')).toEqual([])
    expect(parseAddresses(' , ,\n')).toEqual([])
  })
})
