import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go server mounts the UI at /antennarr/ and serves the API beside it, so
// `npm run dev` proxies the API there. Point ANTENNARR_URL at another instance
// to develop against it.
const backend = process.env.ANTENNARR_URL ?? 'http://127.0.0.1:5004'

export default defineConfig({
  base: '/antennarr/',
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: {
    // Straight into the Go package that embeds it. Not emptied by Vite, which
    // would delete the committed .gitkeep; `npm run clean` does it instead.
    outDir: '../internal/ui/dist',
    emptyOutDir: false,
  },
  // The release notes are CHANGELOG.md at the repository root, bundled into the page (see use-whats-new.ts).
  server: { fs: { allow: ['..'] }, proxy: { '/antennarr/api': backend } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
