import { useSyncExternalStore } from 'react'

/** Whether a CSS media query matches right now, kept up to date as the window changes. False where there is no matchMedia. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      if (typeof window.matchMedia !== 'function') return () => {}
      const mq = window.matchMedia(query)
      mq.addEventListener('change', onChange)
      return () => mq.removeEventListener('change', onChange)
    },
    () => typeof window.matchMedia === 'function' && window.matchMedia(query).matches,
    () => false,
  )
}

/**
 * The phone layout: narrower than Tailwind's `md` breakpoint (768px), where wide tables become stacked cards, or a
 * touch screen too short for a desktop's top bar and dialogs, which is a phone turned sideways (even the biggest are
 * only about 440px high there, and a tablet is more than 700).
 */
export const PHONE_QUERY = '(max-width: 767px), (pointer: coarse) and (max-height: 500px)'
export const useIsPhone = () => useMediaQuery(PHONE_QUERY)
