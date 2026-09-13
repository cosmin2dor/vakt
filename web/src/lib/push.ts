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
