import { describe, expect, it } from 'vitest'
import { isValidSchedulingThresholdRange, parseSchedulingThresholdRange } from '../schedulingThresholdRange'

describe('schedulingThresholdRange', () => {
  it.each([
    [{ min: 90, max: 97, full_monthly_limit: 0 }, true],
    [{ min: 90, max: 90, full_monthly_limit: 0 }, true],
    [{ min: 1, max: 100, full_monthly_limit: 1000 }, true],
    [{ min: 97, max: 90, full_monthly_limit: 0 }, false],
    [{ min: 0, max: 90, full_monthly_limit: 0 }, false],
    [{ min: 90, max: 101, full_monthly_limit: 0 }, false],
    [{ min: 90.5, max: 97, full_monthly_limit: 0 }, false],
    [{ min: 90, max: 97, full_monthly_limit: -1 }, false],
    [{ min: Number.NaN, max: 97, full_monthly_limit: 0 }, false]
  ])('validates %o as %s', (value, expected) => {
    expect(isValidSchedulingThresholdRange(value)).toBe(expected)
  })

  it('parses stored ranges and rejects unusable ones', () => {
    expect(parseSchedulingThresholdRange({ min: 90, max: 97 })).toEqual({ min: 90, max: 97, full_monthly_limit: 0 })
    expect(parseSchedulingThresholdRange({ min: '90', max: '100', full_monthly_limit: '2' })).toEqual({ min: 90, max: 100, full_monthly_limit: 2 })
    expect(parseSchedulingThresholdRange(null)).toBeNull()
    expect(parseSchedulingThresholdRange(95)).toBeNull()
    expect(parseSchedulingThresholdRange({ min: 99, max: 90 })).toBeNull()
  })
})
