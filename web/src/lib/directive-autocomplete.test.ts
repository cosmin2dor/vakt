import { describe, expect, it } from 'vitest'

import { directiveSkeleton, filterDirectives, type Directive } from './directive-autocomplete'

const DIRECTIVES: Directive[] = [
  { name: 'schedule', value_type: 'cron', arity: 1, required: false, system_written: false },
  { name: 'skip_count', value_type: 'integer', arity: 1, required: false, system_written: false },
  { name: 'skip_until', value_type: 'datetime', arity: 1, required: false, system_written: false },
  { name: 'once', value_type: 'datetime', arity: 1, required: false, system_written: false },
]

describe('filterDirectives', () => {
  it('returns everything for an empty query', () => {
    expect(filterDirectives(DIRECTIVES, '')).toHaveLength(4)
  })

  it('matches by case-insensitive prefix', () => {
    expect(filterDirectives(DIRECTIVES, 'sch').map((d) => d.name)).toEqual(['schedule'])
    expect(filterDirectives(DIRECTIVES, 'SKIP').map((d) => d.name)).toEqual([
      'skip_count',
      'skip_until',
    ])
  })

  it('returns nothing when no directive matches', () => {
    expect(filterDirectives(DIRECTIVES, 'zzz')).toEqual([])
  })
})

describe('directiveSkeleton', () => {
  it('inserts an empty-paren skeleton with the cursor between the parens', () => {
    expect(directiveSkeleton('schedule')).toEqual({ text: '@schedule()', cursorOffset: 10 })
  })

  it('places the cursor right after the opening paren regardless of name length', () => {
    const { text, cursorOffset } = directiveSkeleton('id')
    expect(text).toBe('@id()')
    expect(text[cursorOffset - 1]).toBe('(')
    expect(text[cursorOffset]).toBe(')')
  })
})
