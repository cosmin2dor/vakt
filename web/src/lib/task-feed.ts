import type { components } from '@/lib/api-types'

export type Task = components['schemas']['Task']

// next_fire is a resolved timestamp the server already computed (UX.md §6
// card anatomy: "in 2h 14m" is a diff against it) — this is date arithmetic
// on a given instant, not schedule evaluation, so it stays on the client per
// CLAUDE.md's "the backend is the only parser."
export function formatNextFire(nextFire: string, now: Date = new Date()): string {
  const diffMs = new Date(nextFire).getTime() - now.getTime()
  const overdue = diffMs < 0
  const totalMinutes = Math.round(Math.abs(diffMs) / 60_000)

  if (totalMinutes < 1) return overdue ? 'just now' : 'in <1m'

  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60

  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (days === 0 && minutes > 0) parts.push(`${minutes}m`)

  const duration = parts.join(' ')
  return overdue ? `${duration} overdue` : `in ${duration}`
}

export type FeedGroupName = 'Overdue' | 'Today' | 'Upcoming' | 'Triggered' | 'Not scheduled'

export interface FeedGroup {
  name: FeedGroupName
  tasks: Task[]
}

const GROUP_ORDER: FeedGroupName[] = ['Overdue', 'Today', 'Upcoming', 'Triggered', 'Not scheduled']

function isSameLocalDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

// UX.md §6.1's grouping is Overdue / Today / Upcoming / Triggered. Two
// deliberate calls this spec leaves open:
//
// - `triggered` tasks land in their own group regardless of next_fire —
//   they're awaiting fulfillment (SDD.md §3, State reference), not really
//   "scheduled" in the same sense, so a time bucket would misrepresent them.
// - A null next_fire (paused/completed/failed/elapsed-once, SDD.md G11) gets
//   its own trailing "Not scheduled" group rather than sorting to either end
//   of a time-based bucket, since it isn't a time value at all.
export function groupAndSortTasks(tasks: Task[], now: Date = new Date()): FeedGroup[] {
  const buckets = new Map<FeedGroupName, Task[]>(GROUP_ORDER.map((name) => [name, []]))

  for (const task of tasks) {
    if (task.state === 'triggered') {
      buckets.get('Triggered')!.push(task)
      continue
    }
    if (task.next_fire === null) {
      buckets.get('Not scheduled')!.push(task)
      continue
    }

    const fireDate = new Date(task.next_fire)
    if (fireDate.getTime() < now.getTime()) {
      buckets.get('Overdue')!.push(task)
    } else if (isSameLocalDay(fireDate, now)) {
      buckets.get('Today')!.push(task)
    } else {
      buckets.get('Upcoming')!.push(task)
    }
  }

  for (const [name, group] of buckets) {
    group.sort((a, b) => {
      if (a.next_fire === null || b.next_fire === null) {
        return a.title.localeCompare(b.title)
      }
      return new Date(a.next_fire).getTime() - new Date(b.next_fire).getTime()
    })
    buckets.set(name, group)
  }

  return GROUP_ORDER.map((name) => ({ name, tasks: buckets.get(name)! })).filter(
    (group) => group.tasks.length > 0,
  )
}
