import { useEffect, useState } from 'react'

import { parseLine } from '@/lib/parse-client'

const DEBOUNCE_MS = 300

// Wraps a candidate cron string in a synthetic line and asks /parse what it
// means — never evaluated client-side. `cron` is null while there's nothing
// worth previewing yet (e.g. an empty raw-input field).
export function useUpcomingFires(cron: string | null): { fires: string[]; loading: boolean } {
  // Tags the result with the cron it answers, so a stale response never
  // renders once `cron` has moved on — no separate "loading" flag to desync.
  const [result, setResult] = useState<{ cron: string; fires: string[] } | null>(null)

  useEffect(() => {
    if (!cron) return
    let cancelled = false
    const timer = setTimeout(() => {
      parseLine(`@schedule(${cron})`)
        .then((res) => {
          if (!cancelled) setResult({ cron, fires: res.upcoming_fires ?? [] })
        })
        .catch(() => {
          if (!cancelled) setResult({ cron, fires: [] })
        })
    }, DEBOUNCE_MS)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [cron])

  if (!cron) return { fires: [], loading: false }
  return { fires: result?.cron === cron ? result.fires : [], loading: result?.cron !== cron }
}
