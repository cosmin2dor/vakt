import { cn } from 'cn'

import type { Directive } from '@/lib/directive-autocomplete'

// Cursor-anchored dropdown for the "@" trigger. Not a shadcn DropdownMenu —
// that's anchored to a trigger element, not an arbitrary text-cursor pixel
// position, so this is a small positioned popup instead (UX.md's Overlay
// treatment: --popover + border + shadow-md).
export function DirectiveAutocompleteMenu({
  items,
  selectedIndex,
  coords,
  onSelect,
}: {
  items: Directive[]
  selectedIndex: number
  coords: { left: number; top: number }
  onSelect: (name: string) => void
}) {
  return (
    <div
      className="fixed z-50 min-w-40 overflow-hidden rounded-lg border border-border bg-popover p-1 text-popover-foreground shadow-md"
      style={{ left: coords.left, top: coords.top }}
    >
      {items.map((item, i) => (
        <button
          key={item.name}
          type="button"
          // onMouseDown, not onClick — fires before the editor's blur/selection change.
          onMouseDown={(e) => {
            e.preventDefault()
            onSelect(item.name)
          }}
          className={cn(
            'flex w-full items-center justify-between gap-3 rounded-md px-1.5 py-1 text-left font-mono text-sm',
            i === selectedIndex ? 'bg-accent text-accent-foreground' : 'text-popover-foreground',
          )}
        >
          <span>{item.name}</span>
          <span className="text-xs text-muted-foreground">{item.value_type}</span>
        </button>
      ))}
    </div>
  )
}
