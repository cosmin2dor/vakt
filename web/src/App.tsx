import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Toaster } from '@/components/ui/sonner'
import { FirstRunGuide } from '@/components/first-run-guide'
import { TaskFeed } from '@/components/task-feed'
import { VaultBrowser } from '@/components/vault-browser'
import { cn } from '@/lib/utils'

type View = 'feed' | 'vault'

function App() {
  const [view, setView] = useState<View>('feed')

  return (
    <main className="flex min-h-dvh flex-col items-center gap-4 bg-background p-8 text-foreground">
      <h1 className="text-2xl font-semibold tracking-[-0.02em]">Vakt</h1>
      <FirstRunGuide />

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
      <Toaster />
    </main>
  )
}

export default App
