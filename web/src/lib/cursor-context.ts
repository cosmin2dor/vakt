/**
 * Answers exactly one question: which token is the cursor inside, in one
 * line of vault Markdown. Purely syntactic and position-based — no
 * validation, no type coercion, no schedule evaluation. That's the Go
 * parser's job (SDD §2.1: "an authoring aid, not a semantic engine").
 */

export type CursorContext =
  | { kind: 'prose' }
  | { kind: 'after-at'; atPos: number }
  | { kind: 'directive-name'; name: string; start: number; end: number }
  | {
      kind: 'directive-value'
      name: string
      start: number
      end: number
      value: string
    }

// Matches an @name or @name(value) span. Independent of vault-editor's
// highlighter regex — same shape, different concern (cursor position vs.
// "what's in view"), not worth unifying.
const DIRECTIVE_RE = /@([a-zA-Z_][a-zA-Z0-9_]*)(\(([^)]*)\))?/g

// getCursorContext tells you what's under the caret in one line: a
// directive's name, its value, plain prose, or a just-typed bare "@".
// cursorPos is a caret offset into `line` (0 = before the first char).
export function getCursorContext(line: string, cursorPos: number): CursorContext {
  const pos = Math.max(0, Math.min(cursorPos, line.length))

  DIRECTIVE_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = DIRECTIVE_RE.exec(line))) {
    const name = m[1]
    const atPos = m.index
    const nameStart = atPos + 1
    const nameEnd = nameStart + name.length

    if (pos > atPos && pos <= nameEnd) {
      return { kind: 'directive-name', name, start: nameStart, end: nameEnd }
    }

    const value = m[3]
    if (value !== undefined) {
      const valueStart = nameEnd + 1 // past the '('
      const valueEnd = valueStart + value.length
      if (pos > nameEnd && pos <= valueEnd) {
        return { kind: 'directive-value', name, start: valueStart, end: valueEnd, value }
      }
    }
  }

  // Bare "@" just typed, nothing after it yet — the autocomplete trigger.
  if (pos > 0 && line[pos - 1] === '@') {
    return { kind: 'after-at', atPos: pos - 1 }
  }

  return { kind: 'prose' }
}
