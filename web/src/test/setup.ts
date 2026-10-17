import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'

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
