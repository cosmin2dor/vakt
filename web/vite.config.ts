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
  // Dev-only: proxies /api/v1 to a backend. Defaults to the Prism mock
  // (scripts/mock-server.sh), which serves paths as declared in
  // schema/openapi.yaml with no /api/v1 prefix, so it's stripped. Set
  // VITE_API_PROXY_TARGET to point at the real Go daemon (internal/api)
  // instead — it registers routes with the prefix already built in, so
  // nothing is stripped in that case.
  server: {
    proxy: {
      '/api/v1': (() => {
        const target = process.env.VITE_API_PROXY_TARGET || 'http://localhost:4010'
        const isMock = target.includes('localhost:4010')
        return {
          target,
          changeOrigin: true,
          ...(isMock ? { rewrite: (path: string) => path.replace(/^\/api\/v1/, '') } : {}),
        }
      })(),
    },
  },
})
