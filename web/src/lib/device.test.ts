import { describe, expect, it } from 'vitest'
import { deviceModel, deviceTitle } from './device'

const tuner = (friendlyName?: string) => ({ friendlyName })

describe('deviceTitle', () => {
  it('is the name the owner chose', () => {
    expect(deviceTitle({ name: 'antenna', label: 'Living room antenna', tuner: tuner('HDHomeRun FLEX 4K') })).toBe('Living room antenna')
  })
  it('is the model the tuner reports when there is no name, not its key', () => {
    expect(deviceTitle({ name: 'antenna', tuner: tuner('HDHomeRun FLEX 4K') })).toBe('HDHomeRun FLEX 4K')
    expect(deviceTitle({ name: 'antenna', label: '', tuner: tuner('HDHomeRun FLEX 4K') })).toBe('HDHomeRun FLEX 4K')
  })
  it('falls back to the key while the tuner has not been reached', () => {
    expect(deviceTitle({ name: 'antenna', tuner: tuner(undefined) })).toBe('antenna')
    expect(deviceTitle({ name: 'antenna', tuner: tuner('') })).toBe('antenna')
    expect(deviceTitle({ name: 'antenna' })).toBe('antenna')
  })
})

describe('deviceModel', () => {
  it('is the model beside a name the owner chose', () => {
    expect(deviceModel({ name: 'antenna', label: 'Living room antenna', tuner: tuner('HDHomeRun FLEX 4K') })).toBe('HDHomeRun FLEX 4K')
  })
  it('is nothing when the title already is the model, so it is said once', () => {
    expect(deviceModel({ name: 'antenna', tuner: tuner('HDHomeRun FLEX 4K') })).toBeUndefined()
    expect(deviceModel({ name: 'antenna', label: 'HDHomeRun FLEX 4K', tuner: tuner('HDHomeRun FLEX 4K') })).toBeUndefined()
  })
  it('is nothing when the tuner has not said what it is', () => {
    expect(deviceModel({ name: 'antenna', label: 'Living room antenna', tuner: tuner(undefined) })).toBeUndefined()
    expect(deviceModel({ name: 'antenna', label: 'Living room antenna' })).toBeUndefined()
  })
})
