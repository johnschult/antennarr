/** One group of changes in a release: "Added", "Changed", "Read before updating"... Older entries have none, so the title can be null. */
export interface ChangeSection {
  title: string | null
  items: string[]
}

export interface Release {
  /** 2026.10.5 */
  version: string
  /** 2026-10-18, as written in the heading; empty if the heading has none. */
  date: string
  /** Text between the heading and the first group, if any. */
  intro: string | null
  sections: ChangeSection[]
}

/** The same shape the server accepts for a release number: YYYY.MM.PATCH. A build from a checkout ("dev") is not one. */
const CALVER = /^\d{4}\.(0?[1-9]|1[0-2])\.\d+$/
export const isRelease = (v: string): boolean => CALVER.test(v)

/** Orders release numbers, -1, 0 or 1. Two things that are not release numbers compare as equal. */
export function compareVersions(a: string, b: string): number {
  if (!isRelease(a) || !isRelease(b)) return 0
  const x = a.split('.').map(Number)
  const y = b.split('.').map(Number)
  for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return x[i] < y[i] ? -1 : 1
  return 0
}

const RELEASE_HEADING = /^##\s+(\d{4}\.\d{1,2}\.\d+)\s*(?:\((\d{4}-\d{2}-\d{2})\))?\s*$/
const BULLET = /^[-*]\s+(.*)$/

/**
 * Reads CHANGELOG.md: `## 2026.10.5 (2026-10-18)` headings, then optionally a paragraph, then `### Group`
 * headings with `- ` bullets under them (some older releases are bullets alone). The text inside an
 * item is left as written, Markdown and all, for InlineMarkdown to draw. Newest first, as in the file.
 */
export function parseChangelog(text: string): Release[] {
  const lines = text.split(/\r?\n/)
  const releases: Release[] = []
  let release: Release | null = null
  let section: ChangeSection | null = null
  const intro: string[] = []

  const finishIntro = () => {
    if (release && intro.length > 0) release.intro = intro.join(' ')
    intro.length = 0
  }
  const sectionFor = (): ChangeSection => {
    if (!section) {
      finishIntro()
      section = { title: null, items: [] }
      release!.sections.push(section)
    }
    return section
  }

  // A comment is a line (or lines) that starts with <!-- and runs to -->. They are skipped as they are met, not
  // cut out of the text, so there is nothing left over to put a comment back together.
  let inComment = false

  for (const raw of lines) {
    if (inComment) {
      if (raw.includes('-->')) inComment = false
      continue
    }
    const start = raw.trimStart().startsWith('<!--') ? raw.indexOf('<!--') : -1
    if (start >= 0) {
      if (raw.indexOf('-->', start + 4) < 0) inComment = true
      continue
    }
    const heading = RELEASE_HEADING.exec(raw)
    if (heading) {
      finishIntro()
      release = { version: heading[1], date: heading[2] ?? '', intro: null, sections: [] }
      releases.push(release)
      section = null
      continue
    }
    if (!release) continue // the title and the introduction to the whole file

    const line = raw.trimEnd()
    if (line.startsWith('### ')) {
      finishIntro()
      section = { title: line.slice(4).trim(), items: [] }
      release.sections.push(section)
      continue
    }
    if (line.startsWith('## ') || line.startsWith('# ')) continue // some other heading: not part of a release
    if (line.trim() === '') continue

    const bullet = BULLET.exec(line)
    if (bullet) {
      sectionFor().items.push(bullet[1].trim())
    } else if (/^\s+\S/.test(line) && section && section.items.length > 0) {
      section.items[section.items.length - 1] += ' ' + line.trim() // a bullet that carries on to the next line
    } else if (!section) {
      intro.push(line.trim())
    } else {
      section.items.push(line.trim())
    }
  }
  finishIntro()
  return releases
}

/** The releases newer than `seen` and no newer than `current`, newest first, at most `max` of them. */
export function releasesSince(releases: Release[], seen: string, current: string, max = 5): Release[] {
  return releases
    .filter((r) => compareVersions(r.version, seen) > 0 && compareVersions(r.version, current) <= 0)
    .sort((a, b) => compareVersions(b.version, a.version))
    .slice(0, max)
}

/** 2026-10-18 as "18 Oct 2026". Anything else comes back as it is. */
export function releaseDate(date: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  if (!m) return date
  const month = Number(m[2])
  if (month < 1 || month > 12) return date
  return `${Number(m[3])} ${['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'][month - 1]} ${m[1]}`
}
