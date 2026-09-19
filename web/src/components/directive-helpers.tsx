import { useState } from 'react'
import { CalendarClock, Loader2, Timer, Type } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cronShortcutToString, WEEKDAY_LABELS, type CronShortcut } from '@/lib/cron-shortcuts'
import { useUpcomingFires } from '@/lib/use-upcoming-fires'
import type { Directive } from '@/lib/directive-autocomplete'

export interface HelperProps {
  directive: Directive
  value: string
  coords: { left: number; top: number }
  open: boolean
  onOpenChange: (open: boolean) => void
  onCommit: (text: string) => void
}

// PRD §6.2's per-directive helpers, picked by the directive's own ui_helper
// from the registry — never by hard-coding a directive name.
export function DirectiveHelper(props: HelperProps) {
  switch (props.directive.ui_helper) {
    case 'enum_dropdown':
      return <EnumHelper {...props} />
    case 'datetime_picker':
      return <DateTimeHelper {...props} />
    case 'cron_popover':
      return <CronHelper {...props} />
    default:
      return null
  }
}

// @state is a closed registry enum; @target's values come from the running
// dispatch-module registry (schema/directives.yaml's own comment) and has no
// closed list here, so it degrades to a plain text field instead of a Select.
function EnumHelper({ directive, value, coords, open, onOpenChange, onCommit }: HelperProps) {
  if (!directive.values || directive.values.length === 0) {
    return (
      <TextFieldHelper
        icon={<Type />}
        placeholder={directive.name}
        value={value}
        coords={coords}
        open={open}
        onOpenChange={onOpenChange}
        onCommit={onCommit}
      />
    )
  }

  return (
    <Select
      open={open}
      onOpenChange={onOpenChange}
      value={value || undefined}
      onValueChange={onCommit}
    >
      <SelectTrigger
        size="sm"
        className="fixed z-50 bg-popover shadow-md"
        style={{ left: coords.left, top: coords.top }}
      >
        <SelectValue placeholder={directive.name} />
      </SelectTrigger>
      <SelectContent>
        {directive.values.map((v) => (
          <SelectItem key={v} value={v}>
            {v}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

// Small popover + input, for the enum helper's degraded case.
function TextFieldHelper({
  icon,
  placeholder,
  value,
  coords,
  open,
  onOpenChange,
  onCommit,
}: {
  icon: React.ReactNode
  placeholder: string
  value: string
  coords: { left: number; top: number }
  open: boolean
  onOpenChange: (open: boolean) => void
  onCommit: (text: string) => void
}) {
  const [draft, setDraft] = useState(value)

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="secondary"
          size="icon-sm"
          className="fixed z-50"
          style={{ left: coords.left, top: coords.top }}
        >
          {icon}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (draft) onCommit(draft)
          }}
          className="flex gap-1.5"
        >
          <Input
            autoFocus
            placeholder={placeholder}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
          />
          <Button type="submit" size="sm">
            Set
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}

// @once / @skip_until: a native date/time picker (PRD §6.2's literal wording),
// which is also the best fit on mobile. datetime-local's "YYYY-MM-DDTHH:mm"
// output matches one of directive.ParseValue's accepted local layouts.
function DateTimeHelper({ value, coords, open, onOpenChange, onCommit }: HelperProps) {
  const [draft, setDraft] = useState(value)

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="secondary"
          size="icon-sm"
          className="fixed z-50"
          style={{ left: coords.left, top: coords.top }}
        >
          <CalendarClock />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (draft) onCommit(draft)
          }}
          className="flex flex-col gap-2"
        >
          <Input
            autoFocus
            type="datetime-local"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
          />
          <Button type="submit" size="sm" disabled={!draft} className="self-end">
            Insert
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}

// @schedule: shortcut builders that construct a literal cron string via
// substitution only, plus raw input. Never evaluated client-side — the
// preview below comes from POSTing the candidate to /parse.
function CronHelper({ value, coords, open, onOpenChange, onCommit }: HelperProps) {
  const [hour, setHour] = useState(8)
  const [minute, setMinute] = useState(0)
  const [weekday, setWeekday] = useState(1)
  const [tab, setTab] = useState<'daily' | 'weekly' | 'raw'>('daily')
  const [raw, setRaw] = useState(value)

  const shortcut: CronShortcut =
    tab === 'weekly' ? { freq: 'weekly', weekday, hour, minute } : { freq: 'daily', hour, minute }
  const candidate = tab === 'raw' ? raw : cronShortcutToString(shortcut)
  const { fires, loading } = useUpcomingFires(candidate || null)

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="secondary"
          size="icon-sm"
          className="fixed z-50"
          style={{ left: coords.left, top: coords.top }}
        >
          <Timer />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72">
        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList className="w-full">
            <TabsTrigger value="daily">Daily</TabsTrigger>
            <TabsTrigger value="weekly">Weekly</TabsTrigger>
            <TabsTrigger value="raw">Raw</TabsTrigger>
          </TabsList>
          <TabsContent value="daily" className="flex items-center gap-1.5 pt-2">
            <TimeInputs hour={hour} minute={minute} onHour={setHour} onMinute={setMinute} />
          </TabsContent>
          <TabsContent value="weekly" className="flex flex-col gap-2 pt-2">
            <div className="flex flex-wrap gap-1">
              {WEEKDAY_LABELS.map((label, i) => (
                <Button
                  key={label}
                  type="button"
                  size="xs"
                  variant={weekday === i ? 'default' : 'outline'}
                  onClick={() => setWeekday(i)}
                >
                  {label}
                </Button>
              ))}
            </div>
            <TimeInputs hour={hour} minute={minute} onHour={setHour} onMinute={setMinute} />
          </TabsContent>
          <TabsContent value="raw" className="pt-2">
            <Input
              placeholder="0 8 * * *"
              value={raw}
              onChange={(e) => setRaw(e.target.value)}
              className="font-mono"
            />
          </TabsContent>
        </Tabs>
        <div className="mt-2.5 flex flex-col gap-1 border-t border-border pt-2.5">
          <span className="text-xs text-muted-foreground">Next runs</span>
          {loading && <Loader2 className="size-3.5 animate-spin text-muted-foreground" />}
          {!loading && fires.length === 0 && (
            <span className="text-xs text-muted-foreground">No valid schedule yet.</span>
          )}
          {!loading &&
            fires.map((f) => (
              <span key={f} className="text-xs">
                {new Date(f).toLocaleString()}
              </span>
            ))}
        </div>
        <Button
          type="button"
          size="sm"
          className="mt-2.5 w-full"
          disabled={!candidate}
          onClick={() => onCommit(candidate)}
        >
          Insert
        </Button>
      </PopoverContent>
    </Popover>
  )
}

function TimeInputs({
  hour,
  minute,
  onHour,
  onMinute,
}: {
  hour: number
  minute: number
  onHour: (h: number) => void
  onMinute: (m: number) => void
}) {
  return (
    <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
      at
      <Input
        type="number"
        min={0}
        max={23}
        value={hour}
        onChange={(e) => onHour(Number(e.target.value))}
        className="w-14"
      />
      :
      <Input
        type="number"
        min={0}
        max={59}
        value={minute}
        onChange={(e) => onMinute(Number(e.target.value))}
        className="w-14"
      />
    </label>
  )
}
