import type { components } from '@/lib/api-types'

export type EventEnvelope = components['schemas']['EventEnvelope']

const MAX_BACKOFF_MS = 30_000
const RAPID_FAILURE_THRESHOLD = 3

// Subscribes to GET /api/v1/events (SSE). Relies on EventSource's built-in
// reconnect for a single dropped connection; after several rapid failures in
// a row (a backend that's actually down, not a blip) it takes over with its
// own exponential backoff, capped at MAX_BACKOFF_MS, so it doesn't hammer a
// dead daemon. Returns an unsubscribe function for effect cleanup.
export function subscribeToEvents(onEvent: (envelope: EventEnvelope) => void): () => void {
  let source: EventSource | null = null
  let consecutiveFailures = 0
  let backoffTimer: ReturnType<typeof setTimeout> | null = null
  let stopped = false

  function connect() {
    if (stopped) return
    source = new EventSource('/api/v1/events')

    source.onopen = () => {
      consecutiveFailures = 0
    }

    source.onmessage = (e: MessageEvent<string>) => {
      try {
        onEvent(JSON.parse(e.data) as EventEnvelope)
      } catch {
        // malformed payload, ignore
      }
    }

    source.onerror = () => {
      consecutiveFailures += 1
      if (consecutiveFailures < RAPID_FAILURE_THRESHOLD) return

      // Take over from here: close the native retry and back off ourselves.
      source?.close()
      source = null
      const delay = Math.min(
        MAX_BACKOFF_MS,
        1000 * 2 ** (consecutiveFailures - RAPID_FAILURE_THRESHOLD),
      )
      backoffTimer = setTimeout(connect, delay)
    }
  }

  connect()

  return () => {
    stopped = true
    if (backoffTimer) clearTimeout(backoffTimer)
    source?.close()
  }
}
