export interface SchedulingThresholdRangeValue {
  min: number
  max: number
  full_monthly_limit: number
}

export const MAX_FULL_MONTHLY_LIMIT = 1000

export const DEFAULT_SCHEDULING_THRESHOLD_RANGE: SchedulingThresholdRangeValue = {
  min: 90,
  max: 97,
  full_monthly_limit: 0
}

const isIntegerIn = (value: unknown, min: number, max: number) =>
  typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max

// Mirrors the backend rule: 1 <= min <= max <= 100, 0 <= limit <= 1000.
export function isValidSchedulingThresholdRange(value: SchedulingThresholdRangeValue): boolean {
  return (
    isIntegerIn(value.min, 1, 100) &&
    isIntegerIn(value.max, 1, 100) &&
    value.min <= value.max &&
    isIntegerIn(value.full_monthly_limit, 0, MAX_FULL_MONTHLY_LIMIT)
  )
}

// Reads a stored range; anything unusable yields null.
export function parseSchedulingThresholdRange(raw: unknown): SchedulingThresholdRangeValue | null {
  if (!raw || typeof raw !== 'object') return null
  const source = raw as Record<string, unknown>
  const value = {
    min: Number(source.min),
    max: Number(source.max),
    full_monthly_limit: Number(source.full_monthly_limit ?? 0)
  }
  return isValidSchedulingThresholdRange(value) ? value : null
}
