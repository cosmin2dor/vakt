import { useState } from 'react'
import { Check, MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { fulfillTask, pauseTask, resumeTask, skipTask } from '@/lib/task-actions'
import { formatNextFire, type Task } from '@/lib/task-feed'

// UX.md §6 "Task card anatomy" — title + next-fire, @id + file path, state
// chip, and a 2px state-color rail on the left edge. Quick actions (fulfill
// inline, pause/resume/skip in the overflow menu) live in this file per
// implement-quick-actions.
const SUPPRESSION_LABEL: Record<string, string> = {
  state_not_active: 'state not active',
  skip_until: 'skip until',
  early_completion: 'early completion',
  skip_count: 'skip count',
}

// Tailwind needs literal class names to scan for — a template-literal class
// built from `task.state` at runtime would never make it into the compiled
// CSS, so the five rail colors are spelled out here instead.
const RAIL_CLASS: Record<Task['state'], string> = {
  active: 'border-l-state-active',
  triggered: 'border-l-state-triggered',
  paused: 'border-l-state-paused',
  completed: 'border-l-state-completed',
  failed: 'border-l-state-failed',
}

interface TaskCardProps {
  task: Task
  // Optimistic patch to ONE field the action's outcome is unambiguous about
  // (pause -> paused, resume -> active). Rolled back on failure.
  onUpdate: (id: string, patch: Partial<Task>) => void
  // Swap in the server's real Task once a request settles successfully.
  onReplace: (task: Task) => void
}

export function TaskCard({ task, onUpdate, onReplace }: TaskCardProps) {
  // Fulfill/skip can't be guessed correctly without knowing @once vs
  // @schedule shape (or the current skip_count), so instead of asserting a
  // wrong end state we just show them as briefly pending (UX.md §5: no
  // spinners, opacity only) until the real response arrives.
  const [pending, setPending] = useState(false)

  async function handleFulfill() {
    setPending(true)
    try {
      onReplace(await fulfillTask(task.id))
    } catch {
      toast.error(`Couldn't fulfill "${task.title}"`)
    } finally {
      setPending(false)
    }
  }

  async function handleSkip() {
    setPending(true)
    try {
      onReplace(await skipTask(task.id))
    } catch {
      toast.error(`Couldn't skip "${task.title}"`)
    } finally {
      setPending(false)
    }
  }

  // Pause/resume are fully predictable, so the chip flips instantly with no
  // pending affordance at all, and rolls back to the prior state on failure.
  async function handlePause() {
    const previousState = task.state
    onUpdate(task.id, { state: 'paused' })
    try {
      onReplace(await pauseTask(task.id))
    } catch {
      onUpdate(task.id, { state: previousState })
      toast.error(`Couldn't pause "${task.title}"`)
    }
  }

  async function handleResume() {
    const previousState = task.state
    onUpdate(task.id, { state: 'active' })
    try {
      onReplace(await resumeTask(task.id))
    } catch {
      onUpdate(task.id, { state: previousState })
      toast.error(`Couldn't resume "${task.title}"`)
    }
  }

  const canFulfill = task.state === 'active' || task.state === 'triggered'
  const showOverflow = task.state !== 'completed' && task.state !== 'failed'

  return (
    <Card
      size="sm"
      className={`flex-row gap-0 rounded-lg border border-border py-0 ring-0 border-l-2 ${RAIL_CLASS[task.state]} ${pending ? 'opacity-60' : ''}`}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-2 px-4 py-3">
        <div className="flex items-baseline justify-between gap-3">
          <span className="truncate text-[15px] leading-[1.4] font-medium tracking-[-0.005em]">
            {task.title}
          </span>
          {task.next_fire !== null && (
            <span className="shrink-0 font-mono text-[13px] leading-[1.4] tabular-nums text-muted-foreground">
              {formatNextFire(task.next_fire)}
            </span>
          )}
        </div>

        <div className="truncate font-mono text-xs text-muted-foreground">
          @{task.id} &middot; {task.file_path}
        </div>

        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Badge variant={task.state}>{task.state}</Badge>
            {task.effective_suppression.suppressed && (
              <Badge variant="outline" className="text-muted-foreground">
                suppressed
                {task.effective_suppression.reason &&
                  ` · ${SUPPRESSION_LABEL[task.effective_suppression.reason] ?? task.effective_suppression.reason}`}
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-1">
            {canFulfill && (
              <Button variant="ghost" size="sm" disabled={pending} onClick={handleFulfill}>
                <Check /> Fulfill
              </Button>
            )}
            {showOverflow && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-11"
                    disabled={pending}
                    aria-label="More actions"
                  >
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {task.state === 'paused' ? (
                    <DropdownMenuItem onSelect={handleResume}>Resume</DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem onSelect={handlePause}>Pause</DropdownMenuItem>
                  )}
                  <DropdownMenuItem onSelect={handleSkip}>Skip next</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>
      </div>
    </Card>
  )
}
