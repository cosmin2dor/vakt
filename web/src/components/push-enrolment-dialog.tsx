import { useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { hasEnrolledPush, isStandalone, markPushEnrolled, urlBase64ToUint8Array } from '@/lib/push'
import type { components } from '@/lib/api-types'

type Status = 'idle' | 'requesting' | 'denied' | 'success' | 'error'

// UX.md §6, "Push enrolment": plain-language iOS install guidance, no
// celebration on success (§9). Triggered only from a real click — iOS
// requires Notification.requestPermission() to originate in a user gesture.
export function PushEnrolmentDialog({ onEnrolled }: { onEnrolled?: () => void } = {}) {
  // Best-effort local memory of a past success — there's no GET /subscriptions
  // to ask the server, so a returning, already-enrolled user isn't re-prompted
  // to "turn on" something that's already on.
  const [status, setStatus] = useState<Status>(() => (hasEnrolledPush() ? 'success' : 'idle'))
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  async function enable() {
    setStatus('requesting')
    setErrorMessage(null)

    const permission = await Notification.requestPermission()
    if (permission === 'denied') {
      setStatus('denied')
      return
    }
    if (permission === 'default') {
      // User dismissed the prompt without a choice. Leave the dialog open
      // rather than nagging — no dark-pattern re-prompt.
      setStatus('idle')
      return
    }

    try {
      const registration = await navigator.serviceWorker.ready

      const keyResponse = await fetch('/api/v1/vapid-public-key')
      if (!keyResponse.ok) throw new Error('Could not fetch the server key.')
      const { public_key } = (await keyResponse.json()) as { public_key: string }

      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      })

      const body = subscription.toJSON() as components['schemas']['PushSubscription']
      const subscribeResponse = await fetch('/api/v1/subscriptions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!subscribeResponse.ok) throw new Error('The server rejected the subscription.')

      markPushEnrolled()
      setStatus('success')
      onEnrolled?.()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Something went wrong.')
      setStatus('error')
    }
  }

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant={status === 'success' ? 'secondary' : 'default'}>
          {status === 'success' ? 'Notifications on' : 'Enable notifications'}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Notifications</DialogTitle>
          <DialogDescription>
            Enabling notifications lets the household know when something is due — the dog needs
            feeding, the filter needs changing. Nothing else changes.
          </DialogDescription>
        </DialogHeader>

        {!isStandalone() ? (
          <p className="text-sm text-muted-foreground">
            Vakt needs to be added to your Home Screen before it can send notifications. Open the
            share menu and choose &ldquo;Add to Home Screen&rdquo;, then open Vakt from the icon it
            creates and try again.
          </p>
        ) : (
          <>
            {status === 'idle' && <Button onClick={enable}>Turn on notifications</Button>}
            {status === 'requesting' && (
              <p className="text-sm text-muted-foreground">Waiting for a response&hellip;</p>
            )}
            {status === 'success' && (
              <p className="text-sm text-muted-foreground">
                Notifications are on. This device will receive them.
              </p>
            )}
            {status === 'denied' && (
              <p className="text-sm text-muted-foreground">
                Notifications were declined. To turn them on later, open this device&rsquo;s
                Settings, find Vakt, and allow notifications there.
              </p>
            )}
            {status === 'error' && (
              <p className="text-sm text-muted-foreground">
                Notifications could not be enabled. {errorMessage}
              </p>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
