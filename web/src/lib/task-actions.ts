import type { Task } from '@/lib/task-feed'

// Thin client for the quick-action endpoints (schema/openapi.yaml). Each
// posts to a real, merged backend route and returns the server's updated
// Task — the source of truth for optimistic-update reconciliation.
async function post(path: string, body?: unknown): Promise<Task> {
  const res = await fetch(path, {
    method: 'POST',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`POST ${path} -> ${res.status}`)
  return res.json() as Promise<Task>
}

export function fulfillTask(id: string): Promise<Task> {
  return post(`/api/v1/tasks/${id}/fulfill`)
}

export function pauseTask(id: string, reason?: string): Promise<Task> {
  return post(`/api/v1/tasks/${id}/pause`, reason ? { reason } : undefined)
}

export function resumeTask(id: string): Promise<Task> {
  return post(`/api/v1/tasks/${id}/resume`)
}

export function skipTask(id: string, count?: number): Promise<Task> {
  return post(`/api/v1/tasks/${id}/skip`, count ? { count } : undefined)
}
