import { useEffect, useState } from 'react'

import type { Directive } from '@/lib/directive-autocomplete'

// Module-level so every editor instance in the session shares one fetch —
// the registry doesn't change while the app is open.
let cache: Directive[] | null = null
let inflight: Promise<Directive[]> | null = null

function fetchDirectives(): Promise<Directive[]> {
  if (cache) return Promise.resolve(cache)
  inflight ??= fetch('/api/v1/directives')
    .then((res) => {
      if (!res.ok) throw new Error(`GET /api/v1/directives: ${res.status}`)
      return res.json() as Promise<Directive[]>
    })
    .then((data) => {
      cache = data
      return data
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

// Fetches the live directive registry once per session, so autocomplete
// reflects the running daemon's schema/directives.yaml, not a build-time copy.
export function useDirectives(): Directive[] {
  const [directives, setDirectives] = useState<Directive[]>(cache ?? [])

  useEffect(() => {
    if (cache) return
    let cancelled = false
    fetchDirectives()
      .then((data) => {
        if (!cancelled) setDirectives(data)
      })
      .catch(() => {
        // Autocomplete just stays empty; the editor itself is unaffected.
      })
    return () => {
      cancelled = true
    }
  }, [])

  return directives
}
