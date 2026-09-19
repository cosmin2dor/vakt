import { describe, expect, it } from 'vitest'

import { cronShortcutToString } from './cron-shortcuts'

describe('cronShortcutToString', () => {
  it('builds a daily cron string', () => {
    expect(cronShortcutToString({ freq: 'daily', hour: 8, minute: 0 })).toBe('0 8 * * *')
  })

  it('builds a weekly cron string', () => {
    expect(cronShortcutToString({ freq: 'weekly', weekday: 1, hour: 17, minute: 30 })).toBe(
      '30 17 * * 1',
    )
  })

  it('does not zero-pad, matching a literal cron field', () => {
    expect(cronShortcutToString({ freq: 'daily', hour: 6, minute: 5 })).toBe('5 6 * * *')
  })
})
