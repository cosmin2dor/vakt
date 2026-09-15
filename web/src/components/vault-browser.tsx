import { useEffect, useState } from 'react'
import { ChevronRight, File, Folder, FolderOpen } from 'lucide-react'

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import type { components } from '@/lib/api-types'

type DirectoryEntry = components['schemas']['DirectoryEntry']
type FileContent = components['schemas']['FileContent']

type TreeStatus = 'loading' | 'ready' | 'error'
type FileStatus = 'idle' | 'loading' | 'ready' | 'error'

// GET /api/v1/directories: whole vault in one call, per that endpoint's own
// "fetch it all, filter client-side" design (same as the task feed).
export function VaultBrowser() {
  const [status, setStatus] = useState<TreeStatus>('loading')
  const [root, setRoot] = useState<DirectoryEntry | null>(null)
  const [openPath, setOpenPath] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    fetch('/api/v1/directories')
      .then((res) => {
        if (!res.ok) throw new Error(`GET /api/v1/directories -> ${res.status}`)
        return res.json() as Promise<DirectoryEntry>
      })
      .then((data) => {
        if (cancelled) return
        setRoot(data)
        setStatus('ready')
      })
      .catch(() => {
        if (cancelled) return
        setStatus('error')
      })

    return () => {
      cancelled = true
    }
  }, [])

  if (status === 'loading') {
    return <div className="h-24 rounded-lg bg-muted" aria-hidden="true" />
  }

  if (status === 'error') {
    return <p className="text-sm text-muted-foreground">Couldn&rsquo;t load the vault.</p>
  }

  if (!root || !root.children || root.children.length === 0) {
    return <p className="text-sm text-muted-foreground">No files in the vault.</p>
  }

  return (
    <>
      <div className="flex w-full flex-col gap-0.5">
        {root.children.map((entry) => (
          <TreeNode key={entry.path} entry={entry} depth={0} onOpenFile={setOpenPath} />
        ))}
      </div>
      {/* Keyed on path so each file selection remounts with fresh loading state. */}
      <FileDialog key={openPath} path={openPath} onClose={() => setOpenPath(null)} />
    </>
  )
}

function TreeNode({
  entry,
  depth,
  onOpenFile,
}: {
  entry: DirectoryEntry
  depth: number
  onOpenFile: (path: string) => void
}) {
  const [open, setOpen] = useState(false)
  const indent = { paddingLeft: `${depth * 1.25}rem` }

  if (entry.type === 'file') {
    return (
      <button
        type="button"
        onClick={() => onOpenFile(entry.path)}
        style={indent}
        className="flex min-h-11 items-center gap-2 rounded-md pr-2 text-left text-sm hover:bg-muted"
      >
        <File className="size-4 shrink-0 text-muted-foreground" strokeWidth={1.5} />
        <span className="truncate">{entry.name}</span>
      </button>
    )
  }

  const children = entry.children ?? []

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger
        style={indent}
        className="flex min-h-11 w-full items-center gap-2 rounded-md pr-2 text-left text-sm hover:bg-muted"
      >
        <ChevronRight
          className={cn(
            'size-4 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-90',
          )}
          strokeWidth={1.5}
        />
        {open ? (
          <FolderOpen className="size-4 shrink-0 text-muted-foreground" strokeWidth={1.5} />
        ) : (
          <Folder className="size-4 shrink-0 text-muted-foreground" strokeWidth={1.5} />
        )}
        <span className="truncate">{entry.name}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {children.map((child) => (
          <TreeNode key={child.path} entry={child} depth={depth + 1} onOpenFile={onOpenFile} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}

// Fetches GET /api/v1/files?path=... on demand, one file at a time.
function FileDialog({ path, onClose }: { path: string | null; onClose: () => void }) {
  const [status, setStatus] = useState<FileStatus>(path ? 'loading' : 'idle')
  const [file, setFile] = useState<FileContent | null>(null)

  useEffect(() => {
    if (!path) return

    let cancelled = false

    fetch(`/api/v1/files?path=${encodeURIComponent(path)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`GET /api/v1/files -> ${res.status}`)
        return res.json() as Promise<FileContent>
      })
      .then((data) => {
        if (cancelled) return
        setFile(data)
        setStatus('ready')
      })
      .catch(() => {
        if (cancelled) return
        setStatus('error')
      })

    return () => {
      cancelled = true
    }
  }, [path])

  return (
    <Dialog open={path !== null} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="flex max-h-[80dvh] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="truncate font-mono text-sm">{path}</DialogTitle>
        </DialogHeader>
        {status === 'loading' && <div className="h-24 rounded-lg bg-muted" aria-hidden="true" />}
        {status === 'error' && (
          <p className="text-sm text-muted-foreground">Couldn&rsquo;t load this file.</p>
        )}
        {status === 'ready' && file && (
          <pre className="overflow-auto rounded-lg bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
            {file.content}
          </pre>
        )}
      </DialogContent>
    </Dialog>
  )
}
