// Registers the M1 shell's service worker (SDD.md §2.4: iOS only takes push
// subscriptions from a Home Screen PWA whose service worker is registered).
export function registerServiceWorker() {
  if (!('serviceWorker' in navigator)) return

  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.error('Service worker registration failed:', err)
    })
  })
}
