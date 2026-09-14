import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { formatNextFire, type Task } from '@/lib/task-feed'

// UX.md §6 "Task card anatomy" — title + next-fire, @id + file path, state
// chip, and a 2px state-color rail on the left edge. Quick actions
// (fulfill/pause/skip) are explicitly out of scope for this sketch —
// validate-contract-against-mock's job is the card, not the actions.
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

export function TaskCard({ task }: { task: Task }) {
  return (
    <Card
      size="sm"
      className={`flex-row gap-0 rounded-lg border border-border py-0 ring-0 border-l-2 ${RAIL_CLASS[task.state]}`}
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
      </div>
    </Card>
  )
}
