import { describe, expect, it } from 'vitest'
import { PHONE_QUERY } from './use-media-query'

describe('PHONE_QUERY', () => {
  it('is a narrow screen, or a touch screen too short for the desktop layout, which is a phone turned sideways', () => {
    const [narrow, sideways] = PHONE_QUERY.split(',').map((q) => q.trim())
    expect(narrow).toBe('(max-width: 767px)')
    expect(sideways).toContain('(pointer: coarse)') // a mouse on a short window is still a desktop
    expect(sideways).toMatch(/\(max-height: (\d+)px\)/)
    const tallest = Number(/max-height: (\d+)px/.exec(sideways)![1])
    expect(tallest).toBeGreaterThan(440) // an iPhone Pro Max on its side is about 440 high
    expect(tallest).toBeLessThan(700) // a tablet on its side is taller
  })
})
