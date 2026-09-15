import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { PushEnrolmentDialog } from '@/components/push-enrolment-dialog'
import { TaskFeed } from '@/components/task-feed'
import { VaultBrowser } from '@/components/vault-browser'
import { cn } from '@/lib/utils'

// Scaffold placeholder only — proves Vite, Tailwind, shadcn/ui, and the
// design tokens from UX.md are wired correctly end to end. Real screens
// (feed, task card, editor, ...) land in M4/M5. The enrolment dialog is
// wired in here, ahead of any real nav, just to make it reachable.
//
// The feed below is validate-contract-against-mock's sketch, not the real
// M4 dashboard — it renders against schema/openapi.yaml's mock server
// (scripts/mock-server.sh), reachable here since there's no real nav yet.
type View = 'feed' | 'vault'

function App() {
  const [view, setView] = useState<View>('feed')

  return (
    <main className="flex min-h-dvh flex-col items-center gap-4 bg-background p-8 text-foreground">
      <h1 className="text-2xl font-semibold tracking-[-0.02em]">Vakt</h1>
      <p className="text-sm text-muted-foreground">
        Frontend scaffold — Vite, React, TypeScript, Tailwind, shadcn/ui.
      </p>
      <p className="font-mono text-xs text-muted-foreground">@schedule(0 8 * * *)</p>
      <Button>Primary action</Button>
      <PushEnrolmentDialog />

      {/* No router yet — a two-way toggle is enough to reach the vault browser until real nav lands. */}
      <div className="mt-4 flex w-full max-w-md gap-2">
        <Button
          variant={view === 'feed' ? 'secondary' : 'ghost'}
          size="sm"
          className={cn('flex-1')}
          onClick={() => setView('feed')}
        >
          Feed
        </Button>
        <Button
          variant={view === 'vault' ? 'secondary' : 'ghost'}
          size="sm"
          className={cn('flex-1')}
          onClick={() => setView('vault')}
        >
          Vault
        </Button>
      </div>
      <div className="w-full max-w-md">{view === 'feed' ? <TaskFeed /> : <VaultBrowser />}</div>
    </main>
  )
}

export default App
