import { useEffect, useRef } from 'react'
import { EditorState } from '@codemirror/state'
import {
  EditorView,
  Decoration,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view'
import { minimalSetup } from 'codemirror'
import { markdown } from '@codemirror/lang-markdown'

import { DIRECTIVES } from '@/lib/directives-gen'

const SYSTEM_DIRECTIVES = new Set(DIRECTIVES.filter((d) => d.systemWritten).map((d) => d.name))

// UX.md §7 — every value here resolves through a design token (var(--...)),
// never a hex literal, so the editor stays in sync with the rest of the app.
const vaktTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'hsl(var(--background))',
      color: 'hsl(var(--foreground))',
      height: '100%',
      fontFamily: 'var(--font-mono)',
      fontSize: '14px',
    },
    '.cm-scroller': { overflow: 'auto' },
    '.cm-content': { lineHeight: '1.6', caretColor: 'hsl(var(--primary))' },
    '.cm-gutters': {
      backgroundColor: 'hsl(var(--background))',
      color: 'hsl(var(--muted-foreground))',
      border: 'none',
    },
    '.cm-activeLine': { backgroundColor: 'hsl(var(--muted) / 40%)' },
    '.cm-activeLineGutter': { backgroundColor: 'hsl(var(--muted) / 40%)' },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
      backgroundColor: 'hsl(var(--primary) / 18%) !important',
    },
    '.cm-cursor': { borderLeftColor: 'hsl(var(--primary))' },
    '.cm-directive-name': { color: 'hsl(var(--primary))' },
    '.cm-directive-value': { color: 'hsl(var(--foreground))' },
    '.cm-directive-system': { color: 'hsl(var(--muted-foreground))', fontStyle: 'italic' },
    // Stubbed for implement-backend-driven-linting — no decorations use these yet.
    '.cm-diagnostic-error': { textDecoration: 'underline wavy hsl(var(--destructive))' },
    '.cm-diagnostic-warning': { textDecoration: 'underline wavy hsl(var(--state-triggered))' },
  },
  { dark: true },
)

// directiveRe matches an @name or @name(value) span. Simple regex coloring,
// not the semantic cursor-context lexer (that's implement-cursor-context-lexer).
const directiveRe = /@([a-zA-Z_][a-zA-Z0-9_]*)(\(([^)]*)\))?/g

function buildDecorations(view: EditorView): DecorationSet {
  const marks = []
  for (const { from, to } of view.visibleRanges) {
    const text = view.state.doc.sliceString(from, to)
    directiveRe.lastIndex = 0
    let m: RegExpExecArray | null
    while ((m = directiveRe.exec(text))) {
      const name = m[1]
      const matchStart = from + m.index
      const matchEnd = matchStart + m[0].length

      if (SYSTEM_DIRECTIVES.has(name)) {
        marks.push(Decoration.mark({ class: 'cm-directive-system' }).range(matchStart, matchEnd))
        continue
      }

      const nameEnd = matchStart + 1 + name.length
      marks.push(Decoration.mark({ class: 'cm-directive-name' }).range(matchStart, nameEnd))

      const value = m[3]
      if (value) {
        const valueStart = matchEnd - 1 - value.length
        marks.push(
          Decoration.mark({ class: 'cm-directive-value' }).range(
            valueStart,
            valueStart + value.length,
          ),
        )
      }
    }
  }
  return Decoration.set(marks, true)
}

const directiveHighlighter = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = buildDecorations(view)
    }
    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) {
        this.decorations = buildDecorations(update.view)
      }
    }
  },
  { decorations: (v) => v.decorations },
)

export function VaultEditor({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const initialValueRef = useRef(value)

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    if (!containerRef.current) return

    const view = new EditorView({
      state: EditorState.create({
        doc: initialValueRef.current,
        extensions: [
          minimalSetup,
          markdown(),
          EditorView.lineWrapping,
          directiveHighlighter,
          vaktTheme,
          EditorView.updateListener.of((update) => {
            if (update.docChanged) onChangeRef.current(update.state.doc.toString())
          }),
        ],
      }),
      parent: containerRef.current,
    })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // Only the initial value seeds the editor; onChange reports later edits.
  }, [])

  return <div ref={containerRef} className="h-full w-full" />
}
