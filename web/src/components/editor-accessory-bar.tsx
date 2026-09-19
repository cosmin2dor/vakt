import { useLayoutEffect, useRef, type RefObject } from 'react'

import { Button } from '@/components/ui/button'
import { useVisualViewportBottom } from '@/lib/use-visual-viewport-bottom'
import type { VaultEditorHandle } from '@/components/vault-editor'

// PRD §6.2's exact Mobile Quick-Access Bar list: [ ], @, @once, @schedule, @target.
// @ just inserts the character — the autocomplete dropdown is a separate task.
const ACTIONS: { label: string; text: string; cursorOffset?: number }[] = [
  { label: '[ ]', text: '[ ] ' },
  { label: '@', text: '@' },
  { label: '@once', text: '@once()', cursorOffset: 6 },
  { label: '@schedule', text: '@schedule()', cursorOffset: 10 },
  { label: '@target', text: '@target()', cursorOffset: 8 },
]

export function EditorAccessoryBar({
  editorRef,
  onHeightChange,
}: {
  editorRef: RefObject<VaultEditorHandle | null>
  /** Reports the bar's own rendered height so the caller can pad the editor's
   * scroll content — otherwise the last line's caret can end up behind the bar. */
  onHeightChange?: (height: number) => void
}) {
  const keyboardInset = useVisualViewportBottom()
  const barRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!barRef.current || !onHeightChange) return
    const observer = new ResizeObserver(([entry]) => onHeightChange(entry.contentRect.height))
    observer.observe(barRef.current)
    return () => observer.disconnect()
  }, [onHeightChange])

  return (
    <div
      ref={barRef}
      className="fixed inset-x-0 z-50 flex gap-1 border-t border-border bg-background p-1 pb-[max(theme(spacing.1),env(safe-area-inset-bottom))]"
      style={{ bottom: keyboardInset }}
    >
      {ACTIONS.map((action) => (
        <Button
          key={action.label}
          type="button"
          variant="secondary"
          className="h-11 min-w-11 flex-1 font-mono text-sm"
          onClick={() => editorRef.current?.insertAtCursor(action.text, action.cursorOffset)}
        >
          {action.label}
        </Button>
      ))}
    </div>
  )
}
