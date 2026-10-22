import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go server mounts the UI at / and serves the API beside it, so
// `npm run dev` proxies the API there. Point YAGI_URL at another instance
// to develop against it.
const backend = process.env.YAGI_URL ?? 'http://127.0.0.1:9244'

export default defineConfig({
  base: '/',
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: {
    // Straight into the Go package that embeds it. Not emptied by Vite, which
    // would delete the committed .gitkeep; `npm run clean` does it instead.
    outDir: '../internal/ui/dist',
    emptyOutDir: false,
  },
  // The release notes are CHANGELOG.md at the repository root, bundled into the page (see use-whats-new.ts).
  server: { fs: { allow: ['..'] }, proxy: { '/api': backend, '/auth': backend, '/streams': backend } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
