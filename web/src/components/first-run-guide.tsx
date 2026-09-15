import { useState } from 'react'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { PushEnrolmentDialog } from '@/components/push-enrolment-dialog'
import { hasEnrolledPush, isStandalone } from '@/lib/push'

// SDD.md §6 risk: the Home Screen install requirement is a real UX cliff.
// This is the path from "just installed" to "enrolled" spelled out in plain
// language, ahead of the feed, instead of a bare button in scaffold markup.
export function FirstRunGuide() {
  const [enrolled, setEnrolled] = useState(() => hasEnrolledPush())

  if (enrolled) {
    return <p className="text-xs text-muted-foreground">Notifications are on for this device.</p>
  }

  return (
    <Card className="w-full max-w-md">
      <CardHeader>
        <CardTitle>Getting started</CardTitle>
        <CardDescription>
          Add a task to a Markdown file in your vault — it shows up in the feed below once
          it&rsquo;s due.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {!isStandalone() && (
          <p className="text-sm text-muted-foreground">
            To be notified, add Vakt to your Home Screen first: open the share menu and choose
            &ldquo;Add to Home Screen&rdquo;, then open Vakt from that icon.
          </p>
        )}
        <PushEnrolmentDialog onEnrolled={() => setEnrolled(true)} />
      </CardContent>
    </Card>
  )
}
