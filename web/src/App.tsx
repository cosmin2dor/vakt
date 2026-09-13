import { Button } from '@/components/ui/button'
import { PushEnrolmentDialog } from '@/components/push-enrolment-dialog'

// Scaffold placeholder only — proves Vite, Tailwind, shadcn/ui, and the
// design tokens from UX.md are wired correctly end to end. Real screens
// (feed, task card, editor, ...) land in M4/M5. The enrolment dialog is
// wired in here, ahead of any real nav, just to make it reachable.
function App() {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-4 bg-background p-8 text-foreground">
      <h1 className="text-2xl font-semibold tracking-[-0.02em]">Vakt</h1>
      <p className="text-sm text-muted-foreground">
        Frontend scaffold — Vite, React, TypeScript, Tailwind, shadcn/ui.
      </p>
      <p className="font-mono text-xs text-muted-foreground">@schedule(0 8 * * *)</p>
      <Button>Primary action</Button>
      <PushEnrolmentDialog />
    </main>
  )
}

export default App
