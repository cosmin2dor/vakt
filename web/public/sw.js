// Minimal service worker for the M1 PWA shell (build-pwa-shell). Scope here
// is install/registration correctness (SDD.md §2.4), not push templating or
// dispatch — that stays server-side (ios_notifications, a separate task).

// Activate immediately rather than waiting for existing tabs to close.
self.addEventListener('install', () => {
  self.skipWaiting()
})

// Take control of any open clients right after activation.
self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

// Display whatever payload push sends. No templating: title/body/icon are
// placeholders until the notification content is designed (M3).
self.addEventListener('push', (event) => {
  const data = event.data ? event.data.json() : {}
  const title = data.title || 'Vakt'
  const options = {
    body: data.body || '',
    icon: '/icon-192.png',
    badge: '/icon-192.png',
  }
  event.waitUntil(self.registration.showNotification(title, options))
})

// Focus an existing tab if one is open, otherwise open a new one.
self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ('focus' in client) return client.focus()
      }
      if (self.clients.openWindow) return self.clients.openWindow('/')
    }),
  )
})
