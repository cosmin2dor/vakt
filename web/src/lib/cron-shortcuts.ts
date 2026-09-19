/**
 * Pure string templating for the cron popover's "shortcuts" tab. Never
 * interprets cron semantics — just substitutes a couple of UI inputs into a
 * literal 5-field string (CLAUDE.md: the backend is the only parser).
 */

export type CronShortcut =
  | { freq: 'daily'; hour: number; minute: number }
  | { freq: 'weekly'; weekday: number; hour: number; minute: number } // weekday: 0 (Sun) - 6 (Sat), cron's own convention

export function cronShortcutToString(shortcut: CronShortcut): string {
  const minute = String(shortcut.minute)
  const hour = String(shortcut.hour)
  switch (shortcut.freq) {
    case 'daily':
      return `${minute} ${hour} * * *`
    case 'weekly':
      return `${minute} ${hour} * * ${shortcut.weekday}`
  }
}

export const WEEKDAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
