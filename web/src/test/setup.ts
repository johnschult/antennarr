import '@testing-library/jest-dom/vitest'
import * as axeMatchers from 'vitest-axe/matchers'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, expect, vi } from 'vitest'

// toHaveNoViolations, for the accessibility checks in a11y.test.tsx.
expect.extend(axeMatchers)

// Nothing in a test may reach a real network: unless a test stubs fetch itself, every call is refused.
beforeEach(() => {
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ error: 'no network in tests' }), { status: 503 }))
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  // each test starts with no saved theme and a bare <html>
  localStorage.clear()
  document.documentElement.className = ''
  document.documentElement.style.colorScheme = ''
})

// jsdom has no pointer capture, which Vaul's drag handle calls when a sheet is pressed.
for (const m of ['setPointerCapture', 'releasePointerCapture'] as const) {
  if (!(m in Element.prototype)) Object.defineProperty(Element.prototype, m, { value: () => {}, configurable: true })
}
if (!('hasPointerCapture' in Element.prototype)) Object.defineProperty(Element.prototype, 'hasPointerCapture', { value: () => false, configurable: true })
