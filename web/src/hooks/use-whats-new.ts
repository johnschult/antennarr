import { useCallback, useEffect, useMemo, useState } from 'react'
import { compareVersions, isRelease, parseChangelog, releasesSince, type Release } from '@/lib/changelog'

/** Where this browser remembers the last release whose notes it showed. */
export const SEEN_KEY = 'antennarr.lastSeenVersion'

/** Reads the release notes. They are bundled into the page from CHANGELOG.md, as their own file that is only fetched when needed. */
export type LoadChangelog = () => Promise<string>
const bundled: LoadChangelog = () => import('../../../CHANGELOG.md?raw').then((m) => m.default)

// null: nothing remembered. undefined: this browser won't let the page remember anything.
function readSeen(): string | null | undefined {
  try {
    return localStorage.getItem(SEEN_KEY)
  } catch {
    return undefined
  }
}
function writeSeen(version: string) {
  try {
    localStorage.setItem(SEEN_KEY, version)
  } catch {
    // nowhere to remember it; readSeen says so, and then the notes are not shown by themselves
  }
}

/**
 * Shows what is new after an update. It opens by itself when this is a release (not a build from a checkout) and
 * a different, older one was the last this browser saw: with the notes of everything since, newest first. A
 * browser that has never been here records the version and shows nothing, since the history is of no use to a
 * new install; and one that can't remember anything never opens it by itself, rather than every time.
 * `show` opens the notes for this release on request, from the footer.
 */
export function useWhatsNew(current: string, load: LoadChangelog = bundled) {
  const release = isRelease(current)
  const [seen, setSeen] = useState(readSeen)
  const [manual, setManual] = useState(false)
  const [text, setText] = useState<string | null>(null)

  // Updated: a release, and an older one was the last seen. (A first visit, or a build older than the last seen,
  // is not an update: there is nothing to show, so it only records this version.)
  const updated = release && typeof seen === 'string' && compareVersions(seen, current) < 0
  useEffect(() => {
    if (release && seen !== undefined && !updated && seen !== current) writeSeen(current)
  }, [release, seen, updated, current])
  const wanted = updated || manual

  useEffect(() => {
    if (!wanted || text !== null) return
    let cancelled = false
    load().then(
      (t) => !cancelled && setText(t),
      () => !cancelled && setText(''), // can't read them: there is nothing to show
    )
    return () => {
      cancelled = true
    }
  }, [wanted, text, load])

  const all = useMemo(() => (text ? parseChangelog(text) : []), [text])
  const releases: Release[] = useMemo(() => {
    if (updated && typeof seen === 'string') return releasesSince(all, seen, current)
    const latest = all.find((r) => r.version === current) ?? all.find((r) => compareVersions(r.version, current) <= 0)
    return latest ? [latest] : []
  }, [all, updated, seen, current])

  // Updated, but the notes have nothing for it: remember this version so it isn't tried at every visit.
  useEffect(() => {
    if (updated && text !== null && releases.length === 0) writeSeen(current)
  }, [updated, text, releases.length, current])

  const dismiss = useCallback(() => {
    if (release) writeSeen(current)
    setSeen(release ? current : readSeen())
    setManual(false)
  }, [release, current])
  const show = useCallback(() => setManual(true), [])

  return { open: wanted && releases.length > 0, releases, current, byItself: updated, dismiss, show, canShow: release }
}
