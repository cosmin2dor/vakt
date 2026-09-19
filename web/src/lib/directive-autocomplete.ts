/**
 * Pure logic for the @-trigger autocomplete: which directives match what's
 * been typed, and what skeleton text a selection inserts. No CodeMirror or
 * DOM here — that's vault-editor.tsx's job.
 */

import type { components } from '@/lib/api-types'

export type Directive = components['schemas']['Directive']

// Case-insensitive prefix match against the partial name typed after "@".
export function filterDirectives(directives: Directive[], query: string): Directive[] {
  const q = query.toLowerCase()
  return directives.filter((d) => d.name.toLowerCase().startsWith(q))
}

// "@name()" with the cursor placed between the parens — the same skeleton
// shape the mobile accessory bar's own directive buttons insert.
export function directiveSkeleton(name: string): { text: string; cursorOffset: number } {
  const text = `@${name}()`
  return { text, cursorOffset: text.length - 1 }
}
