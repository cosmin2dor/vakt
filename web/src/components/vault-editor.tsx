import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
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
import { getCursorContext, type CursorContext } from '@/lib/cursor-context'
import { directiveSkeleton, filterDirectives, type Directive } from '@/lib/directive-autocomplete'
import { useDirectives } from '@/lib/use-directives'
import { DirectiveAutocompleteMenu } from '@/components/directive-autocomplete-menu'
import { DirectiveHelper } from '@/components/directive-helpers'

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
    '.cm-content': {
      lineHeight: '1.6',
      caretColor: 'hsl(var(--primary))',
      // Keeps the last line's caret clear of the mobile accessory bar (UX.md §8).
      paddingBottom: 'var(--vakt-editor-bottom-inset, 0px)',
      scrollMarginBottom: 'var(--vakt-editor-bottom-inset, 0px)',
    },
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

// Imperative API for callers (e.g. the mobile accessory bar) that need to
// insert text at the caret without reaching into CodeMirror internals.
export interface VaultEditorHandle {
  /** Inserts text at the cursor. cursorOffsetInText places the cursor inside
   * the inserted text (e.g. between the parens of "@once()") instead of after it. */
  insertAtCursor: (text: string, cursorOffsetInText?: number) => void
}

export const VaultEditor = forwardRef<
  VaultEditorHandle,
  {
    value: string
    onChange: (value: string) => void
    onCursorContextChange?: (ctx: CursorContext) => void
    /** Extra bottom padding/scroll-margin so the accessory bar never covers the caret. */
    bottomInset?: number
  }
>(function VaultEditor({ value, onChange, onCursorContextChange, bottomInset = 0 }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const onCursorContextChangeRef = useRef(onCursorContextChange)
  const initialValueRef = useRef(value)

  // "@" autocomplete: anchorRef is the doc position of the triggering "@";
  // autocomplete is non-null while the user is still typing that directive's name.
  const anchorRef = useRef<number | null>(null)
  const [autocomplete, setAutocomplete] = useState<{
    query: string
    selectedIndex: number
    coords: { left: number; top: number; bottom: number }
  } | null>(null)
  const directives = useDirectives()

  // Contextual helper (PRD §6.2): shown whenever the cursor sits inside a
  // directive's value and that directive has a ui_helper. `open` toggles
  // between the small trigger (icon/select) and its expanded content.
  const [activeHelper, setActiveHelper] = useState<{
    directive: Directive
    from: number
    to: number
    value: string
    coords: { left: number; top: number }
    open: boolean
  } | null>(null)
  // Doc position of a value just inserted by autocomplete, so the very next
  // cursor-context update auto-opens its helper instead of showing a trigger.
  const autoOpenPosRef = useRef<number | null>(null)

  function closeAutocomplete() {
    anchorRef.current = null
    setAutocomplete(null)
  }

  function commitAutocomplete(name: string) {
    const view = viewRef.current
    if (!view || anchorRef.current === null) return
    const from = anchorRef.current
    const to = Math.max(from, view.state.selection.main.head)
    const { text, cursorOffset } = directiveSkeleton(name)
    const directive = directives.find((d) => d.name === name)
    if (directive?.ui_helper) autoOpenPosRef.current = from + cursorOffset
    view.dispatch({
      changes: { from, to, insert: text },
      selection: { anchor: from + cursorOffset },
    })
    view.focus()
    closeAutocomplete()
  }

  function commitHelperValue(text: string) {
    const view = viewRef.current
    if (!view || !activeHelper) return
    const { from, to } = activeHelper
    view.dispatch({
      changes: { from, to, insert: text },
      selection: { anchor: from + text.length },
    })
    view.focus()
    setActiveHelper(null)
  }

  function handleAutocompleteKeyDown(e: React.KeyboardEvent) {
    if (!autocomplete) return
    const items = filterDirectives(directives, autocomplete.query)
    if (items.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      e.stopPropagation()
      setAutocomplete((prev) =>
        prev ? { ...prev, selectedIndex: (prev.selectedIndex + 1) % items.length } : prev,
      )
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      e.stopPropagation()
      setAutocomplete((prev) =>
        prev
          ? { ...prev, selectedIndex: (prev.selectedIndex - 1 + items.length) % items.length }
          : prev,
      )
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      e.stopPropagation()
      commitAutocomplete(items[Math.min(autocomplete.selectedIndex, items.length - 1)].name)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      closeAutocomplete()
    }
  }

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useImperativeHandle(ref, () => ({
    insertAtCursor(text, cursorOffsetInText) {
      const view = viewRef.current
      if (!view) return
      const { from, to } = view.state.selection.main
      const anchor = from + (cursorOffsetInText ?? text.length)
      view.dispatch({
        changes: { from, to, insert: text },
        selection: { anchor },
      })
      view.focus()
    },
  }))

  useEffect(() => {
    onCursorContextChangeRef.current = onCursorContextChange
  }, [onCursorContextChange])

  // Read inside the mount-only editor effect below, which can't depend on
  // the (async-loaded) directives array without remounting CodeMirror.
  const directivesRef = useRef(directives)
  useEffect(() => {
    directivesRef.current = directives
  }, [directives])

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
            if (update.docChanged || update.selectionSet) {
              const head = update.state.selection.main.head
              const line = update.state.doc.lineAt(head)
              const ctx = getCursorContext(line.text, head - line.from)
              onCursorContextChangeRef.current?.(ctx)

              if (ctx.kind === 'after-at') {
                const atPos = line.from + ctx.atPos
                anchorRef.current = atPos
                const coords = update.view.coordsAtPos(atPos)
                if (coords) setAutocomplete({ query: '', selectedIndex: 0, coords })
              } else if (
                ctx.kind === 'directive-name' &&
                anchorRef.current !== null &&
                line.from + ctx.start - 1 === anchorRef.current
              ) {
                const coords = update.view.coordsAtPos(anchorRef.current)
                if (coords) setAutocomplete({ query: ctx.name, selectedIndex: 0, coords })
              } else {
                anchorRef.current = null
                setAutocomplete(null)
              }

              if (ctx.kind === 'directive-value') {
                const directive = directivesRef.current.find((d) => d.name === ctx.name)
                const from = line.from + ctx.start
                const to = line.from + ctx.end
                const coords = update.view.coordsAtPos(to)
                if (directive?.ui_helper && coords) {
                  const autoOpen = autoOpenPosRef.current === from
                  autoOpenPosRef.current = null
                  setActiveHelper((prev) => ({
                    directive,
                    from,
                    to,
                    value: ctx.value,
                    coords: { left: coords.left, top: coords.bottom + 4 },
                    open: autoOpen || (prev?.from === from ? prev.open : false),
                  }))
                } else {
                  setActiveHelper(null)
                }
              } else {
                setActiveHelper(null)
              }
            }
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

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.dom.style.setProperty('--vakt-editor-bottom-inset', `${bottomInset}px`)
  }, [bottomInset])

  const autocompleteItems = autocomplete ? filterDirectives(directives, autocomplete.query) : []

  return (
    <div className="h-full w-full" onKeyDownCapture={handleAutocompleteKeyDown}>
      {/* CodeMirror mounts its own DOM here imperatively — React never renders children into it. */}
      <div ref={containerRef} className="h-full w-full" />
      {autocomplete && autocompleteItems.length > 0 && (
        <DirectiveAutocompleteMenu
          items={autocompleteItems}
          selectedIndex={Math.min(autocomplete.selectedIndex, autocompleteItems.length - 1)}
          coords={{ left: autocomplete.coords.left, top: autocomplete.coords.bottom + 4 }}
          onSelect={commitAutocomplete}
        />
      )}
      {activeHelper && (
        <DirectiveHelper
          key={activeHelper.from}
          directive={activeHelper.directive}
          value={activeHelper.value}
          coords={activeHelper.coords}
          open={activeHelper.open}
          onOpenChange={(open) => setActiveHelper((prev) => (prev ? { ...prev, open } : prev))}
          onCommit={commitHelperValue}
        />
      )}
    </div>
  )
})
