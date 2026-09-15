// Web Push boilerplate: PushManager.subscribe wants applicationServerKey as
// raw bytes, but the server hands back a base64url string (SDD.md §2.3).
export function urlBase64ToUint8Array(base64Url: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64Url.length % 4)) % 4)
  const base64 = (base64Url + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  return Uint8Array.from(raw, (char) => char.charCodeAt(0))
}

// iOS only accepts a push subscription from a PWA already added to the Home
// Screen (SDD.md §2.4). Covers both the standard media query and the older
// iOS Safari `navigator.standalone` flag.
export function isStandalone(): boolean {
  return (
    window.matchMedia('(display-mode: standalone)').matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  )
}

// No GET /subscriptions endpoint exists to ask the server "has this device
// already enrolled" (schema/openapi.yaml only has POST create/unsubscribe),
// so this device's own enrolment history is tracked locally instead.
const ENROLLED_KEY = 'vakt.push-enrolled'

export function hasEnrolledPush(): boolean {
  try {
    return localStorage.getItem(ENROLLED_KEY) === '1'
  } catch {
    return false
  }
}

export function markPushEnrolled(): void {
  try {
    localStorage.setItem(ENROLLED_KEY, '1')
  } catch {
    // Best-effort — private browsing can throw on write.
  }
}
