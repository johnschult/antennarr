import { categoryLooks, languageName } from '@/lib/channel-about'
import type { TunerChannel } from '@/lib/types'

/** The filters on a channel list beyond its state. Within a group a channel needs any one of those chosen; it needs every group that has any. */
export interface Facets {
  /** Only channels in high definition. */
  hd: boolean
  /** Category labels, as categoryLooks gives them. */
  categories: ReadonlySet<string>
  /** Language codes. */
  languages: ReadonlySet<string>
}

export const NO_FACETS: Facets = { hd: false, categories: new Set(), languages: new Set() }

/** How many things are being filtered on. */
export const facetCount = (f: Facets): number => (f.hd ? 1 : 0) + f.categories.size + f.languages.size

/** A channel's category labels, each once. */
export const categoriesOf = (c: TunerChannel): string[] => categoryLooks(c.about?.categories).map((l) => l.label)

/** Whether a channel passes the filters. A channel the guide says nothing about can't pass a category or language filter: nothing is assumed of it. */
export function passes(c: TunerChannel, f: Facets): boolean {
  if (f.hd && !c.hd) return false
  if (f.categories.size > 0 && !categoriesOf(c).some((l) => f.categories.has(l))) return false
  if (f.languages.size > 0 && !(c.about?.languages ?? []).some((l) => f.languages.has(l))) return false
  return true
}

export interface Counted {
  key: string
  label: string
  count: number
}

/** What there is to filter on in a list of channels, with how many channels each is on, most first. */
export function facetOptions(channels: TunerChannel[]): { hd: number; categories: Counted[]; languages: Counted[]; unknown: number } {
  const cats = new Map<string, number>()
  const langs = new Map<string, number>()
  let hd = 0
  let unknown = 0
  for (const c of channels) {
    if (c.hd) hd++
    const cs = categoriesOf(c)
    const ls = c.about?.languages ?? []
    if (cs.length === 0 && ls.length === 0) unknown++
    for (const l of cs) cats.set(l, (cats.get(l) ?? 0) + 1)
    for (const l of ls) langs.set(l, (langs.get(l) ?? 0) + 1)
  }
  const order = (a: Counted, b: Counted) => b.count - a.count || a.label.localeCompare(b.label)
  return {
    hd,
    unknown,
    categories: [...cats].map(([key, count]) => ({ key, label: key, count })).sort(order),
    languages: [...langs].map(([key, count]) => ({ key, label: languageName(key), count })).sort(order),
  }
}
