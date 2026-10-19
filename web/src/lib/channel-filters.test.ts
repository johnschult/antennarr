import { describe, expect, it } from 'vitest'
import { facetCount, facetOptions, NO_FACETS, passes } from './channel-filters'
import type { TunerChannel } from './types'

const ch = (number: string, over: Partial<TunerChannel> = {}): TunerChannel => ({ number, name: number, state: 'enabled', subscribed: true, ...over })
const a = ch('2.1', { hd: true, about: { languages: ['en'], categories: ['News', 'Talk'] } })
const b = ch('2.2', { about: { languages: ['en', 'es'], categories: ['Sitcom'] } })
const c = ch('26.1', { hd: true, about: { languages: ['es'], categories: ['Shop', 'Shopping'] } })
const none = ch('9.1') // the guide doesn't have it

const facets = (over: Partial<{ hd: boolean; categories: string[]; languages: string[] }>) => ({ hd: over.hd ?? false, categories: new Set(over.categories), languages: new Set(over.languages) })

describe('passes', () => {
  it('lets everything through when nothing is chosen', () => {
    expect([a, b, c, none].every((x) => passes(x, NO_FACETS))).toBe(true)
  })
  it('keeps only HD when asked', () => {
    expect([a, b, c, none].filter((x) => passes(x, facets({ hd: true }))).map((x) => x.number)).toEqual(['2.1', '26.1'])
  })
  it('takes any one of the categories chosen, by the look they are shown as', () => {
    expect([a, b, c].filter((x) => passes(x, facets({ categories: ['Comedy'] }))).map((x) => x.number)).toEqual(['2.2']) // Sitcom shows as Comedy
    expect([a, b, c].filter((x) => passes(x, facets({ categories: ['News', 'Shopping'] }))).map((x) => x.number)).toEqual(['2.1', '26.1'])
  })
  it('needs every group that has a choice', () => {
    expect([a, b, c].filter((x) => passes(x, facets({ hd: true, languages: ['es'] }))).map((x) => x.number)).toEqual(['26.1'])
    expect([a, b, c].filter((x) => passes(x, facets({ categories: ['News'], languages: ['es'] })))).toEqual([])
  })
  it('leaves out a channel the guide says nothing about from a category or language filter, and never assumes one', () => {
    expect(passes(none, facets({ categories: ['News'] }))).toBe(false)
    expect(passes(none, facets({ languages: ['en'] }))).toBe(false)
    expect(passes(none, facets({ hd: true }))).toBe(false) // it is not HD either
  })
})

describe('facetOptions', () => {
  const o = facetOptions([a, b, c, none])
  it('counts the channels each choice is on, most first, each look once', () => {
    expect(o.hd).toBe(2)
    expect(o.categories.map((x) => [x.key, x.count])).toEqual([
      ['Comedy', 1],
      ['News', 1],
      ['Shopping', 1],
      ['Talk', 1],
    ]) // Sitcom shows as Comedy; Shop and Shopping are one
    expect(o.languages.map((x) => [x.key, x.label, x.count])).toEqual([
      ['en', 'English', 2],
      ['es', 'Spanish', 2],
    ])
  })
  it('counts the channels the guide has nothing on', () => {
    expect(o.unknown).toBe(1)
  })
})

describe('facetCount', () => {
  it('counts what is on', () => {
    expect(facetCount(NO_FACETS)).toBe(0)
    expect(facetCount(facets({ hd: true, categories: ['News', 'Talk'], languages: ['en'] }))).toBe(4)
  })
})
