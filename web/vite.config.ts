import path from 'node:path'

import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  // Production build output. The Go binary (internal/api, per SDD.md §2.3/§2.6)
  // will eventually serve this directory as the static PWA bundle from the
  // same origin as the API — no wiring is done on the Go side yet, but this
  // is the path a later task should point its static file server at.
  build: {
    outDir: 'dist',
  },
})
