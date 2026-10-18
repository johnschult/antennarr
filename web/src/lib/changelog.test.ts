import { describe, expect, it } from 'vitest'
import changelog from '../../../CHANGELOG.md?raw'
import { compareVersions, isRelease, parseChangelog, releaseDate, releasesSince } from './changelog'

describe('the real CHANGELOG.md', () => {
  // The notes pop up in the app, so each release has to be in a shape the page can read.
  const releases = parseChangelog(changelog)

  it('has every release, newest first, each with a number and a date', () => {
    expect(releases.length).toBeGreaterThanOrEqual(6)
    for (const r of releases) {
      expect(isRelease(r.version), r.version).toBe(true)
      expect(r.date, r.version).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    }
    for (let i = 1; i < releases.length; i++) {
      expect(compareVersions(releases[i - 1].version, releases[i].version), `${releases[i - 1].version} before ${releases[i].version}`).toBe(1)
    }
  })

  it('gives every release something to say, with no empty items', () => {
    for (const r of releases) {
      const items = r.sections.flatMap((s) => s.items)
      expect(items.length, r.version).toBeGreaterThan(0)
      for (const item of items) expect(item.trim(), r.version).not.toBe('')
    }
  })

  it('puts "Read before updating" first when a release has it', () => {
    for (const r of releases) {
      const i = r.sections.findIndex((s) => s.title === 'Read before updating')
      if (i >= 0) expect(i, r.version).toBe(0)
    }
  })

  it('only links to http or https addresses, in full, since the notes are also a GitHub release page', () => {
    for (const r of releases) {
      for (const item of r.sections.flatMap((s) => s.items)) {
        for (const m of item.matchAll(/\]\(([^)\s]+)\)/g)) expect(m[1], `${r.version}: ${m[1]}`).toMatch(/^https?:\/\//)
      }
    }
  })
})

describe('parseChangelog', () => {
  const sample = `# Changelog

<!-- a note
that goes over two lines -->

Intro to the whole file.

## 2026.10.5 (2026-10-18)

An introduction to this release,
over two lines.

### Read before updating

- **First** thing, with a [link](https://example.com).
- Second thing
  that carries on.

### Added

- Third *one*.

## 2026.10.1 (2026-10-17)

- Bullets with no group.
- Another.

## 2026.10.0 (2026-10-17)
`
  const r = parseChangelog(sample)

  it('reads each release, ignoring the file title, its introduction and comments', () => {
    expect(r.map((x) => x.version)).toEqual(['2026.10.5', '2026.10.1', '2026.10.0'])
    expect(r[0].date).toBe('2026-10-18')
  })
  it('reads the introduction, the groups and their items', () => {
    expect(r[0].intro).toBe('An introduction to this release, over two lines.')
    expect(r[0].sections.map((s) => s.title)).toEqual(['Read before updating', 'Added'])
    expect(r[0].sections[0].items).toEqual(['**First** thing, with a [link](https://example.com).', 'Second thing that carries on.'])
    expect(r[0].sections[1].items).toEqual(['Third *one*.'])
  })
  it('puts bullets with no group in one untitled group', () => {
    expect(r[1].sections).toEqual([{ title: null, items: ['Bullets with no group.', 'Another.'] }])
    expect(r[1].intro).toBeNull()
  })
  it('keeps a release with nothing under it, so the caller can decide', () => {
    expect(r[2].sections).toEqual([])
  })
  it('copes with an empty file, Windows line endings and a heading with no date', () => {
    expect(parseChangelog('')).toEqual([])
    expect(parseChangelog('## 2026.10.2\r\n\r\n- Hello\r\n')).toEqual([{ version: '2026.10.2', date: '', intro: null, sections: [{ title: null, items: ['Hello'] }] }])
  })
  it('skips comments wherever they are, on one line or several, and keeps what is around them', () => {
    const r = parseChangelog('<!-- one line -->\n## 2026.10.5 (2026-10-18)\n\n- Before\n<!-- a comment\nover two lines\n- Not an item\n## 2026.10.4 (2026-10-17)\n-->\n- After\n')
    expect(r.map((x) => x.version)).toEqual(['2026.10.5'])
    expect(r[0].sections[0].items).toEqual(['Before', 'After'])
  })
  it('does not put a comment back together out of the pieces of two', () => {
    // Cutting out the inner comment of this would leave a new <!-- behind; it is just text, shown as it is.
    const r = parseChangelog('## 2026.10.5 (2026-10-18)\n\n- <!<!-- x -->-- not a comment\n')
    expect(r[0].sections[0].items).toEqual(['<!<!-- x -->-- not a comment'])
  })
  it('stops at an unterminated comment rather than reading on as if it were not there', () => {
    const r = parseChangelog('## 2026.10.5 (2026-10-18)\n\n- Kept\n<!-- never closed\n- Gone\n')
    expect(r[0].sections[0].items).toEqual(['Kept'])
  })
  it('ignores text that is not a release heading', () => {
    expect(parseChangelog('## Unreleased\n\n- Not yet\n')).toEqual([])
  })
})

describe('versions', () => {
  it('knows a release number from a dev build', () => {
    for (const v of ['2026.10.0', '2026.1.12', '2027.12.100']) expect(isRelease(v), v).toBe(true)
    for (const v of ['dev', 'dev+3553344', '2026.13.0', '2026.10', 'v2026.10.0', '']) expect(isRelease(v), v).toBe(false)
  })
  it('orders them by number, not as text', () => {
    expect(compareVersions('2026.10.4', '2026.10.5')).toBe(-1)
    expect(compareVersions('2026.10.10', '2026.10.9')).toBe(1)
    expect(compareVersions('2026.9.9', '2026.10.0')).toBe(-1)
    expect(compareVersions('2027.1.0', '2026.12.30')).toBe(1)
    expect(compareVersions('2026.10.5', '2026.10.5')).toBe(0)
    expect(compareVersions('dev', '2026.10.5')).toBe(0)
  })
})

describe('releasesSince', () => {
  const all = ['2026.10.5', '2026.10.4', '2026.10.3', '2026.10.2', '2026.10.1', '2026.10.0'].map((version) => ({ version, date: '', intro: null, sections: [] }))
  const versions = (seen: string, current: string, max?: number) => releasesSince(all, seen, current, max).map((r) => r.version)
  it('is what came after the one last seen, up to this one, newest first', () => {
    expect(versions('2026.10.3', '2026.10.5')).toEqual(['2026.10.5', '2026.10.4'])
    expect(versions('2026.10.4', '2026.10.5')).toEqual(['2026.10.5'])
  })
  it('leaves out anything newer than this build', () => {
    expect(versions('2026.10.1', '2026.10.3')).toEqual(['2026.10.3', '2026.10.2'])
  })
  it('is nothing when nothing is new', () => {
    expect(versions('2026.10.5', '2026.10.5')).toEqual([])
  })
  it('stops at the most it was asked for, keeping the newest', () => {
    expect(versions('2026.9.0', '2026.10.5', 3)).toEqual(['2026.10.5', '2026.10.4', '2026.10.3'])
  })
})

describe('releaseDate', () => {
  it('writes a date the way people read it', () => {
    expect(releaseDate('2026-10-18')).toBe('18 Oct 2026')
    expect(releaseDate('2026-01-05')).toBe('5 Jan 2026')
  })
  it('hands back what it cannot read', () => {
    expect(releaseDate('')).toBe('')
    expect(releaseDate('2026-13-01')).toBe('2026-13-01')
    expect(releaseDate('soon')).toBe('soon')
  })
})
