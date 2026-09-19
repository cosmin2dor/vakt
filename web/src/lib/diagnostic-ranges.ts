import type { ParseDiagnostic } from '@/lib/parse-client'

export interface LineInfo {
  /** Absolute document offset of this line's first character. */
  from: number
  text: string
}

export interface DiagnosticRange {
  from: number
  to: number
  severity: ParseDiagnostic['severity']
}

// Splits a document into LineInfo, tracking each line's absolute offset —
// pure so it's testable without a CodeMirror view.
export function computeLineOffsets(doc: string): LineInfo[] {
  const lines: LineInfo[] = []
  let offset = 0
  for (const text of doc.split('\n')) {
    lines.push({ from: offset, text })
    offset += text.length + 1
  }
  return lines
}

// /parse's diagnostics are byte offsets into the single line submitted
// (internal/api/parse.go). This converts each line's diagnostics into
// absolute document ranges for CodeMirror decorations, clamping to the
// line's own bounds in case the line text and diagnostics went stale.
export function toDiagnosticRanges(
  lines: LineInfo[],
  byLine: ParseDiagnostic[][],
): DiagnosticRange[] {
  const ranges: DiagnosticRange[] = []
  for (let i = 0; i < lines.length; i++) {
    const diagnostics = byLine[i]
    if (!diagnostics) continue
    const { from, text } = lines[i]
    for (const d of diagnostics) {
      const start = Math.max(0, Math.min(d.start, text.length))
      const end = Math.max(start, Math.min(d.end, text.length))
      if (end <= start) continue
      ranges.push({ from: from + start, to: from + end, severity: d.severity })
    }
  }
  return ranges
}
