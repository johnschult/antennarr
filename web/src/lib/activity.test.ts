import { describe, expect, it } from 'vitest'
import type { HistorySession } from './types'
import { channelLabel, dailyBytes, dailyMinutes, dataThroughAntennarr, dayLabel, formatBytes, groupByDay, minutesOf, receptionByChannel, sessionSpan, spanText, summarize, totalsByChannel, totalsByWatcher, watcherOf } from './activity'

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

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [900, '900 B'],
    [4_500, '5 KB'],
    [800_000, '800 KB'],
    [1_500_000, '1.5 MB'],
    [12_400_000, '12 MB'],
    [1_400_000_000, '1.4 GB'],
    [25_000_000_000, '25 GB'],
  ])('%i bytes is %s', (n, text) => expect(formatBytes(n)).toBe(text))
})

describe('data', () => {
  const GB = 1_000_000_000
  const withBytes = (over: Partial<HistorySession>) => session(local(2026, 10, 19, 20), local(2026, 10, 19, 21), over)

  it('is split between days by how long a session was on each', () => {
    // 3 hours across midnight: 1 hour on the 18th, 2 on the 19th, so a third and two thirds
    const s = session(local(2026, 10, 18, 23), local(2026, 10, 19, 2), { bytes: 3 * GB })
    const days = dailyBytes([s], 3, now)
    expect(days.map((d) => d.bytes)).toEqual([0, GB, 2 * GB])
  })

  it('has one entry per day, oldest first, and counts only what was measured', () => {
    const days = dailyBytes([withBytes({ bytes: 2 * GB }), withBytes({ bytes: undefined })], 7, now)
    expect(days).toHaveLength(7)
    expect(days[6].bytes).toBe(2 * GB)
    expect(days[0].bytes).toBe(0)
  })

  it('counts only what went out through Antennarr as the sharer’s upload', () => {
    const sessions = [withBytes({ bytes: 2 * GB }), withBytes({ kind: 'remote', viewer: undefined, bytes: GB }), withBytes({ kind: 'local', viewer: undefined, bytes: 5 * GB })]
    expect(dataThroughAntennarr(sessions)).toBe(3 * GB)
  })
})

describe('totals', () => {
  const GB = 1_000_000_000
  const h = (n: number) => local(2026, 10, 19, n)

  it('add up each viewer’s time and data, most data first, and say what stayed on your network', () => {
    const rows = totalsByWatcher([
      session(h(8), h(9), { viewer: 'Alex', bytes: 2 * GB }),
      session(h(10), h(12), { viewer: 'Sam', bytes: 5 * GB }),
      session(h(13), h(14), { viewer: 'Alex', bytes: GB }),
      session(h(15), h(16), { kind: 'local', viewer: undefined, bytes: 9 * GB }),
    ])
    expect(rows.map((r) => [r.label, r.bytes, r.sessions, r.through])).toEqual([
      ['Your devices', 9 * GB, 1, false],
      ['Sam', 5 * GB, 1, true],
      ['Alex', 3 * GB, 2, true],
    ])
    expect(rows[2].ms).toBe(2 * 3_600_000)
  })

  it('give a channel its average quality, weighted by how long it was watched, and none when never reported', () => {
    const rows = totalsByChannel([
      session(h(8), h(9), { quality: 100 }), // one hour at 100
      session(h(10), h(13), { quality: 60 }), // three hours at 60
      session(h(14), h(15), { channel: '5.1', channelName: 'WFRV', quality: undefined }),
      session(h(16), h(17), { channel: '5.1', channelName: 'WFRV', quality: 0 }),
    ])
    expect(rows.map((r) => r.label)).toEqual(['2.1 WSB-HD', '5.1 WFRV']) // most time first
    expect(rows[0].quality).toBe(70) // (100*1 + 60*3) / 4
    expect(rows[1].quality).toBeUndefined()
  })
})

describe('receptionByChannel', () => {
  const h = (n: number) => local(2026, 10, 19, n)

  it('averages the quality of a channel on one tuner, weighted by how long it was watched', () => {
    const got = receptionByChannel(
      [
        session(h(8), h(9), { quality: 100 }), // one hour at 100
        session(h(10), h(13), { quality: 60 }), // three hours at 60
        session(h(14), h(15), { channel: '5.1', channelName: 'WFRV', quality: 40 }),
        session(h(16), h(17), { device: 'other', quality: 10 }), // another tuner's 2.1 is not this one's
      ],
      'antenna',
    )
    expect(got.get('2.1')).toBe(70)
    expect(got.get('5.1')).toBe(40)
    expect(got.size).toBe(2)
  })

  it('has nothing for a channel that was never watched, or whose quality the tuner never gave', () => {
    const got = receptionByChannel([session(h(8), h(9), { quality: undefined }), session(h(10), h(11), { quality: 0 })], 'antenna')
    expect(got.size).toBe(0)
  })
})
