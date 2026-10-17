import { vi } from 'vitest'

export interface FakeWindow {
  closed: boolean
  opener: unknown
  location: { href: string }
  close: ReturnType<typeof vi.fn>
}

/** A stand-in for the pop-up window that window.open returns. */
export function fakeWindow(): FakeWindow {
  const w: FakeWindow = {
    closed: false,
    opener: 'the opener',
    location: { href: '' },
    close: vi.fn(() => {
      w.closed = true
    }),
  }
  return w
}

/** Make window.open return `w` (or null, as a pop-up blocker would). */
export function stubOpen(w: FakeWindow | null) {
  return vi.spyOn(window, 'open').mockImplementation(() => w as unknown as Window | null)
}
