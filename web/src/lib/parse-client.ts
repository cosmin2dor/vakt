import type { components } from '@/lib/api-types'

export type ParseResult = components['schemas']['ParseResult']
export type ParseDiagnostic = components['schemas']['ParseDiagnostic']

// POST /api/v1/parse: the sole authority for what a line of vault text means
// (CLAUDE.md). Used by the contextual helpers' previews — never a client-side
// cron/date evaluation.
export async function parseLine(line: string): Promise<ParseResult> {
  const res = await fetch('/api/v1/parse', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ line }),
  })
  if (!res.ok) throw new Error(`POST /api/v1/parse -> ${res.status}`)
  return res.json() as Promise<ParseResult>
}
