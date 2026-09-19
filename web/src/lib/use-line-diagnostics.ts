import { useEffect, useState } from 'react'

import { parseLine, type ParseDiagnostic } from '@/lib/parse-client'

const DEBOUNCE_MS = 300

export interface DocumentDiagnostics {
  /** Diagnostics per line (0-indexed), byte offsets relative to that line's own text. */
  byLine: ParseDiagnostic[][]
  /** True when /parse was unreachable for this pass — "we don't know", never "it's clean". */
  offline: boolean
}

// Whole-document lint (issues/milestone5.md implement-backend-driven-linting):
// /parse is line-scoped (internal/api/parse.go), so one call per line, fired
// as a debounced batch after each doc change and aggregated. Tags the result
// with the doc it answers (use-upcoming-fires.ts's pattern) so a late batch
// never overwrites a newer request's result.
export function useLineDiagnostics(doc: string): DocumentDiagnostics {
  const [result, setResult] = useState<{ doc: string } & DocumentDiagnostics>({
    doc: '',
    byLine: [],
    offline: false,
  })

  useEffect(() => {
    let cancelled = false
    const timer = setTimeout(() => {
      const lines = doc.split('\n')
      Promise.all(
        lines.map((line) =>
          parseLine(line)
            .then((res) => ({ ok: true as const, diagnostics: res.diagnostics ?? [] }))
            .catch(() => ({ ok: false as const, diagnostics: [] as ParseDiagnostic[] })),
        ),
      ).then((outcomes) => {
        if (cancelled) return
        setResult({
          doc,
          byLine: outcomes.map((o) => o.diagnostics),
          offline: outcomes.some((o) => !o.ok),
        })
      })
    }, DEBOUNCE_MS)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [doc])

  if (result.doc !== doc) return { byLine: [], offline: false }
  return { byLine: result.byLine, offline: result.offline }
}
