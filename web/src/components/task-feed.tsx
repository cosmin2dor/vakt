import { useEffect, useState } from 'react'

import { Separator } from '@/components/ui/separator'
import { TaskCard } from '@/components/task-card'
import { groupAndSortTasks, type Task } from '@/lib/task-feed'

type Status = 'loading' | 'ready' | 'error'

// validate-contract-against-mock: fetches GET /api/v1/tasks from the
// contract-generated mock server (scripts/mock-server.sh) and renders the
// aggregated feed per UX.md §6/§6.1. This is a sketch proving the contract,
// not the real M4 dashboard — no quick actions, no vault browser.
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
