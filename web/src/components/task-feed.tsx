import { useEffect, useState } from 'react'

import { Separator } from '@/components/ui/separator'
import { TaskCard } from '@/components/task-card'
import { groupAndSortTasks, type Task } from '@/lib/task-feed'
import { subscribeToEvents } from '@/lib/events'

type Status = 'loading' | 'ready' | 'error'

// wire-feed-to-real-api: fetches GET /api/v1/tasks from the real daemon
// (dev/vite.config.ts proxies to it, or to the Prism mock by default) and
// renders the aggregated feed per UX.md §6/§6.1. Quick actions and the
// vault browser are separate, later M4 tasks.
//
// implement-live-updates-via-sse: the initial fetch only gives a snapshot;
// GET /api/v1/events then keeps `tasks` in sync as the vault changes.
export function TaskFeed() {
  const [status, setStatus] = useState<Status>('loading')
  const [tasks, setTasks] = useState<Task[]>([])

  useEffect(() => {
    let cancelled = false

    fetch('/api/v1/tasks')
      .then((res) => {
        if (!res.ok) throw new Error(`GET /api/v1/tasks -> ${res.status}`)
        return res.json() as Promise<Task[]>
      })
      .then((data) => {
        if (cancelled) return
        setTasks(data)
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

  useEffect(() => {
    return subscribeToEvents((envelope) => {
      if (envelope.type === 'task_upserted' && envelope.task) {
        const upserted = envelope.task
        setTasks((prev) => {
          const idx = prev.findIndex((t) => t.id === upserted.id)
          if (idx === -1) return [...prev, upserted]
          const next = [...prev]
          next[idx] = upserted
          return next
        })
      } else if (envelope.type === 'task_removed' && envelope.task_id) {
        const removedId = envelope.task_id
        setTasks((prev) => prev.filter((t) => t.id !== removedId))
      }
      // diagnostic_raised/diagnostic_cleared/heartbeat: no feed-level action.
    })
  }, [])

  if (status === 'loading') {
    // UX.md §5 — no skeleton shimmer, a static muted block instead.
    return <div className="h-24 rounded-lg bg-muted" aria-hidden="true" />
  }

  if (status === 'error') {
    return <p className="text-sm text-muted-foreground">Couldn&rsquo;t load the feed.</p>
  }

  const groups = groupAndSortTasks(tasks)

  if (groups.length === 0) {
    return <p className="text-sm text-muted-foreground">No tasks in the vault.</p>
  }

  return (
    <div className="flex w-full flex-col gap-8">
      {groups.map((group, i) => (
        <section key={group.name} className="flex flex-col gap-2">
          {i > 0 && <Separator className="mb-6" />}
          <h2 className="text-xs font-medium tracking-[0.08em] text-muted-foreground uppercase">
            {group.name}
          </h2>
          <div className="flex flex-col gap-2">
            {group.tasks.map((task) => (
              <TaskCard key={task.id} task={task} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
