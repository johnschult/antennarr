import { describe, expect, it } from 'vitest'
import type { HistorySession } from './types'
import { channelLabel, dailyMinutes, dayLabel, groupByDay, minutesOf, sessionSpan, spanText, summarize, watcherOf } from './activity'

// Built from local dates, so the days mean the same wherever the tests run.
const local = (y: number, mo: number, d: number, h = 0, mi = 0, s = 0) => new Date(y, mo - 1, d, h, mi, s)
const session = (start: Date, end: Date, over: Partial<HistorySession> = {}): HistorySession => ({
  device: 'antenna', tuner: 'tuner0', channel: '2.1', channelName: 'WSB-HD', kind: 'viewer', viewer: 'Alex', start: start.toISOString(), end: end.toISOString(), ...over,
})
const now = local(2026, 10, 19, 21, 0)

describe('spanText', () => {
  it.each([
    [45_000, '45s'],
    [60_000, '1m'],
    [12 * 60_000, '12m'],
    [95 * 60_000, '1h 35m'],
    [125 * 60_000, '2h 05m'],
    [0, '1s'],
  ])('%i ms is %s', (ms, text) => expect(spanText(ms)).toBe(text))
})

describe('sessionSpan and minutesOf', () => {
  it('say how long a session ran, and count at least a minute', () => {
    const s = session(local(2026, 10, 19, 20, 0), local(2026, 10, 19, 21, 35))
    expect(sessionSpan(s)).toBe('1h 35m')
    expect(minutesOf(s)).toBe(95)
    expect(minutesOf(session(local(2026, 10, 19, 20, 0), local(2026, 10, 19, 20, 0, 20)))).toBe(1)
  })
})

describe('dayLabel', () => {
  it('names today and yesterday, and dates the rest', () => {
    expect(dayLabel(local(2026, 10, 19), now)).toBe('Today')
    expect(dayLabel(local(2026, 10, 18), now)).toBe('Yesterday')
    expect(dayLabel(local(2026, 10, 12), now)).toMatch(/Oct/)
    expect(dayLabel(local(2026, 10, 12), now)).toMatch(/12/)
  })
})

describe('groupByDay', () => {
  it('puts sessions under the day they began, newest first', () => {
    const a = session(local(2026, 10, 19, 8), local(2026, 10, 19, 9))
    const b = session(local(2026, 10, 19, 20), local(2026, 10, 19, 21))
    const c = session(local(2026, 10, 18, 23), local(2026, 10, 19, 1)) // began yesterday, ran past midnight
    const groups = groupByDay([a, c, b])
    expect(groups.map((g) => g.day.getDate())).toEqual([19, 18])
    expect(groups[0].sessions).toEqual([b, a])
    expect(groups[1].sessions).toEqual([c])
  })

  it('is empty for nothing', () => expect(groupByDay([])).toEqual([]))
})

describe('dailyMinutes', () => {
  it('has one entry per day, oldest first, ending today', () => {
    const days = dailyMinutes([], 7, now)
    expect(days).toHaveLength(7)
    expect(days[6].day.getDate()).toBe(19)
    expect(days[0].day.getDate()).toBe(13)
    expect(days.every((d) => d.minutes === 0)).toBe(true)
  })

  it('adds up what was watched each day', () => {
    const days = dailyMinutes([session(local(2026, 10, 19, 20, 0), local(2026, 10, 19, 21, 30)), session(local(2026, 10, 19, 8, 0), local(2026, 10, 19, 8, 30))], 3, now)
    expect(days.map((d) => d.minutes)).toEqual([0, 0, 120])
  })

  it('counts a session past midnight on both days', () => {
    const days = dailyMinutes([session(local(2026, 10, 18, 23, 0), local(2026, 10, 19, 1, 30))], 3, now)
    expect(days.map((d) => d.minutes)).toEqual([0, 60, 90])
  })

  it('leaves out what is before the range', () => {
    expect(dailyMinutes([session(local(2026, 10, 1, 20), local(2026, 10, 1, 21))], 3, now).every((d) => d.minutes === 0)).toBe(true)
  })
})

describe('summarize', () => {
  it('finds the total, the most watched channel and the biggest watcher', () => {
    const s = summarize([
      session(local(2026, 10, 19, 8), local(2026, 10, 19, 9)),
      session(local(2026, 10, 19, 10), local(2026, 10, 19, 10, 30), { channel: '5.1', channelName: 'WFRV' }),
      session(local(2026, 10, 19, 11), local(2026, 10, 19, 13), { channel: '5.1', channelName: 'WFRV', kind: 'local', viewer: undefined, client: '192.168.1.5' }),
    ])
    expect(s.totalMs).toBe(3.5 * 3_600_000)
    expect(s.sessions).toBe(3)
    expect(s.topChannel).toEqual({ label: '5.1 WFRV', ms: 2.5 * 3_600_000 })
    expect(s.topWatcher).toEqual({ label: 'Your devices', ms: 2 * 3_600_000 })
  })

  it('has no top anything for nothing', () => {
    const s = summarize([])
    expect(s).toEqual({ totalMs: 0, sessions: 0, topChannel: undefined, topWatcher: undefined })
  })
})

describe('labels', () => {
  it('names who watched', () => {
    expect(watcherOf(session(now, now))).toBe('Alex')
    expect(watcherOf(session(now, now, { kind: 'viewer', viewer: undefined }))).toBe('Remote')
    expect(watcherOf(session(now, now, { kind: 'remote' }))).toBe('Remote')
    expect(watcherOf(session(now, now, { kind: 'local' }))).toBe('Your devices')
  })

  it('shows a channel by number and name, or the number alone', () => {
    expect(channelLabel(session(now, now))).toBe('2.1 WSB-HD')
    expect(channelLabel(session(now, now, { channelName: undefined }))).toBe('2.1')
  })
})
