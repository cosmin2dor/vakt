import { describe, expect, it } from 'vitest'

import { getCursorContext } from './cursor-context'

describe('getCursorContext', () => {
  it('finds the cursor inside a directive name', () => {
    const line = '@schedule(0 8 * * *)'
    const ctx = getCursorContext(line, 4) // "@sch|edule(...)"
    expect(ctx).toEqual({ kind: 'directive-name', name: 'schedule', start: 1, end: 9 })
  })

  it('finds the cursor inside a directive value', () => {
    const line = '@schedule(0 8 * * *)'
    const pos = line.indexOf('8', line.indexOf('(')) + 1 // "@schedule(0 8| * * *)"
    expect(getCursorContext(line, pos)).toEqual({
      kind: 'directive-value',
      name: 'schedule',
      start: 10,
      end: 19,
      value: '0 8 * * *',
    })
  })

  it('treats text before a directive as prose', () => {
    const line = 'Buy milk @schedule(0 8 * * *)'
    expect(getCursorContext(line, 4)).toEqual({ kind: 'prose' })
  })

  it('treats text after a directive as prose', () => {
    const line = '@schedule(0 8 * * *) done'
    expect(getCursorContext(line, 24)).toEqual({ kind: 'prose' })
  })

  it('treats text between two directives as prose', () => {
    const line = '@id(chore) and @schedule(0 8 * * *)'
    expect(getCursorContext(line, 12)).toEqual({ kind: 'prose' })
  })

  it('reports after-at right after a bare @', () => {
    const line = 'Buy milk @'
    expect(getCursorContext(line, 10)).toEqual({ kind: 'after-at', atPos: 9 })
  })

  it('reports after-at for @ followed by an invalid name char', () => {
    const line = '@2fa reminder'
    expect(getCursorContext(line, 1)).toEqual({ kind: 'after-at', atPos: 0 })
  })

  it('handles the start of the line', () => {
    expect(getCursorContext('hello world', 0)).toEqual({ kind: 'prose' })
  })

  it('handles the end of the line', () => {
    const line = 'hello world'
    expect(getCursorContext(line, line.length)).toEqual({ kind: 'prose' })
  })

  it('handles an empty line without crashing', () => {
    expect(getCursorContext('', 0)).toEqual({ kind: 'prose' })
  })

  it('clamps out-of-range cursor positions', () => {
    expect(getCursorContext('hi', -5)).toEqual({ kind: 'prose' })
    expect(getCursorContext('hi', 999)).toEqual({ kind: 'prose' })
  })

  it('treats checkbox and title prose as prose', () => {
    const line = '- [ ] Buy milk @schedule(0 8 * * *)'
    expect(getCursorContext(line, 2)).toEqual({ kind: 'prose' }) // inside "[ ]"
    expect(getCursorContext(line, 10)).toEqual({ kind: 'prose' }) // "Buy milk"
  })

  it('attributes the cursor to whichever of multiple directives it is inside', () => {
    const line = '@id(chore) @schedule(0 8 * * *) @once(2026-01-01)'

    const idPos = line.indexOf('id') + 1
    expect(getCursorContext(line, idPos)).toEqual({
      kind: 'directive-name',
      name: 'id',
      start: 1,
      end: 3,
    })

    const schedStart = line.indexOf('@schedule')
    const valuePos = line.indexOf('8', line.indexOf('(', schedStart)) + 1
    expect(getCursorContext(line, valuePos)).toEqual({
      kind: 'directive-value',
      name: 'schedule',
      start: schedStart + 10,
      end: schedStart + 19,
      value: '0 8 * * *',
    })

    const oncePos = line.indexOf('once') + 2
    expect(getCursorContext(line, oncePos)).toEqual({
      kind: 'directive-name',
      name: 'once',
      start: line.indexOf('once'),
      end: line.indexOf('once') + 4,
    })
  })

  it('is directive-name right at the boundary before the opening paren', () => {
    const line = '@schedule(0 8 * * *)'
    const pos = line.indexOf('(')
    expect(getCursorContext(line, pos)).toEqual({
      kind: 'directive-name',
      name: 'schedule',
      start: 1,
      end: 9,
    })
  })

  it('is directive-value right at the boundary before the closing paren', () => {
    const line = '@schedule(0 8 * * *)'
    const pos = line.indexOf(')')
    expect(getCursorContext(line, pos)).toEqual({
      kind: 'directive-value',
      name: 'schedule',
      start: 10,
      end: 19,
      value: '0 8 * * *',
    })
  })
})
