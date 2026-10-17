import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(() => {
  cleanup()
  // each test starts with no saved theme and a bare <html>
  localStorage.clear()
  document.documentElement.className = ''
  document.documentElement.style.colorScheme = ''
})
