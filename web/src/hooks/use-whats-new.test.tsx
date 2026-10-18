import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SEEN_KEY, useWhatsNew } from './use-whats-new'

afterEach(() => vi.restoreAllMocks())

const notes = `# Changelog

## 2026.10.5 (2026-10-18)

### Read before updating

- Five

## 2026.10.4 (2026-10-18)

### Changed

- Four

## 2026.10.3 (2026-10-17)

- Three

## 2026.10.2 (2026-10-17)

- Two

## 2026.10.1 (2026-10-17)

- One

## 2026.10.0 (2026-10-16)

- Zero
`
const loader = () => vi.fn().mockResolvedValue(notes)
const seen = () => localStorage.getItem(SEEN_KEY)
const wait = (ms = 30) => act(() => new Promise((r) => setTimeout(r, ms)))

describe('useWhatsNew', () => {
  it('says nothing to a browser that has never been here, and remembers the version without loading any notes', async () => {
    const load = loader()
    const { result } = renderHook(() => useWhatsNew('2026.10.5', load))
    await wait()
    expect(result.current.open).toBe(false)
    expect(seen()).toBe('2026.10.5')
    expect(load).not.toHaveBeenCalled()
  })

  it('says nothing when this is the version it last saw', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.5')
    const load = loader()
    const { result } = renderHook(() => useWhatsNew('2026.10.5', load))
    await wait()
    expect(result.current.open).toBe(false)
    expect(load).not.toHaveBeenCalled()
  })

  it('opens by itself after an update, with the notes for what came after the last one seen', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.4')
    const load = loader()
    const { result } = renderHook(() => useWhatsNew('2026.10.5', load))
    await waitFor(() => expect(result.current.open).toBe(true))
    expect(result.current.byItself).toBe(true)
    expect(result.current.releases.map((r) => r.version)).toEqual(['2026.10.5'])
    expect(load).toHaveBeenCalledTimes(1)
    expect(seen()).toBe('2026.10.4') // not until it is dismissed
  })

  it('shows every release that went by, newest first, and no more than five', async () => {
    localStorage.setItem(SEEN_KEY, '2026.9.0')
    const { result } = renderHook(() => useWhatsNew('2026.10.5', loader()))
    await waitFor(() => expect(result.current.open).toBe(true))
    expect(result.current.releases.map((r) => r.version)).toEqual(['2026.10.5', '2026.10.4', '2026.10.3', '2026.10.2', '2026.10.1'])
  })

  it('stays away until dismissed, then remembers this version and does not open again', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.3')
    const load = loader()
    const first = renderHook(() => useWhatsNew('2026.10.5', load))
    await waitFor(() => expect(first.result.current.open).toBe(true))
    act(() => first.result.current.dismiss())
    expect(first.result.current.open).toBe(false)
    expect(seen()).toBe('2026.10.5')
    first.unmount()

    const second = renderHook(() => useWhatsNew('2026.10.5', load))
    await wait()
    expect(second.result.current.open).toBe(false)
  })

  it('opens again if the page is reloaded without dismissing it', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.4')
    renderHook(() => useWhatsNew('2026.10.5', loader())).unmount()
    const again = renderHook(() => useWhatsNew('2026.10.5', loader()))
    await waitFor(() => expect(again.result.current.open).toBe(true))
  })

  it('never opens by itself for a build from a checkout, and writes nothing', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.1')
    const load = loader()
    for (const v of ['dev', 'dev+3553344']) {
      const { result, unmount } = renderHook(() => useWhatsNew(v, load))
      await wait()
      expect(result.current.open, v).toBe(false)
      expect(result.current.canShow, v).toBe(false)
      unmount()
    }
    expect(seen()).toBe('2026.10.1')
    expect(load).not.toHaveBeenCalled()
  })

  it('does not treat going back to an older build as an update, and remembers it', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.5')
    const load = loader()
    const { result } = renderHook(() => useWhatsNew('2026.10.3', load))
    await wait()
    expect(result.current.open).toBe(false)
    expect(seen()).toBe('2026.10.3')
    expect(load).not.toHaveBeenCalled()
  })

  it('never opens by itself in a browser that will not remember anything, but still opens when asked', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { result } = renderHook(() => useWhatsNew('2026.10.5', loader()))
    await wait()
    expect(result.current.open).toBe(false)
    act(() => result.current.show())
    await waitFor(() => expect(result.current.open).toBe(true))
    act(() => result.current.dismiss()) // must not throw
    expect(result.current.open).toBe(false)
  })

  it('shows nothing, and does not fail, when the notes cannot be read', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.4')
    const load = vi.fn().mockRejectedValue(new Error('no'))
    const { result } = renderHook(() => useWhatsNew('2026.10.5', load))
    await wait()
    expect(result.current.open).toBe(false)
  })

  it('shows nothing when the notes have nothing for this version, and remembers it so it is not tried at every visit', async () => {
    localStorage.setItem(SEEN_KEY, '2026.9.0')
    const { result } = renderHook(() => useWhatsNew('2027.1.0', vi.fn().mockResolvedValue('## 2026.8.0 (2026-08-01)\n\n- Old\n')))
    await wait()
    expect(result.current.open).toBe(false)
    expect(seen()).toBe('2027.1.0')
  })

  it('opens the notes for this release on request, whatever was seen', async () => {
    localStorage.setItem(SEEN_KEY, '2026.10.5')
    const { result } = renderHook(() => useWhatsNew('2026.10.5', loader()))
    expect(result.current.open).toBe(false)
    act(() => result.current.show())
    await waitFor(() => expect(result.current.open).toBe(true))
    expect(result.current.byItself).toBe(false)
    expect(result.current.releases.map((r) => r.version)).toEqual(['2026.10.5'])
    act(() => result.current.dismiss())
    expect(result.current.open).toBe(false)
  })
})
