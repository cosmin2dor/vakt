import { describe, expect, it } from 'vitest'

import { computeLineOffsets, toDiagnosticRanges } from './diagnostic-ranges'

describe('computeLineOffsets', () => {
  it('tracks each line’s absolute start offset', () => {
    expect(computeLineOffsets('abc\nde\nf')).toEqual([
      { from: 0, text: 'abc' },
      { from: 4, text: 'de' },
      { from: 7, text: 'f' },
    ])
  })
})

describe('toDiagnosticRanges', () => {
  it('offsets a line-relative diagnostic into absolute document positions', () => {
    const lines = computeLineOffsets('ok\n@skip_count(not-a-number)')
    const ranges = toDiagnosticRanges(lines, [
      [],
      [{ severity: 'error', message: 'bad value', start: 12, end: 24 }],
    ])
    expect(ranges).toEqual([{ from: 15, to: 27, severity: 'error' }])
  })

  it('handles diagnostics on multiple lines', () => {
    const lines = computeLineOffsets('@bogus\n@id(x)')
    const ranges = toDiagnosticRanges(lines, [
      [{ severity: 'warning', message: 'unknown', start: 0, end: 6 }],
      [{ severity: 'error', message: 'bad id', start: 4, end: 5 }],
    ])
    expect(ranges).toEqual([
      { from: 0, to: 6, severity: 'warning' },
      { from: 11, to: 12, severity: 'error' },
    ])
  })

  it('clamps a stale diagnostic to the line’s current bounds', () => {
    const lines = computeLineOffsets('short')
    const ranges = toDiagnosticRanges(lines, [
      [{ severity: 'error', message: 'stale', start: 3, end: 999 }],
    ])
    expect(ranges).toEqual([{ from: 3, to: 5, severity: 'error' }])
  })

  it('skips lines with no diagnostics entry', () => {
    const lines = computeLineOffsets('a\nb')
    expect(toDiagnosticRanges(lines, [[]])).toEqual([])
  })
})
