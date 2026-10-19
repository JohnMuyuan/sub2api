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

export type SchedulingThresholdWindowMode = 'fixed' | 'random'

export interface SchedulingThresholdWindowRule {
  mode: SchedulingThresholdWindowMode
  threshold?: number
  min?: number
  max?: number
  full_monthly_limit?: number
}

export type SchedulingThresholdWindowRules = Record<string, SchedulingThresholdWindowRule>

// Mirrors AccountSchedulingThresholdPlatformWindows in the backend.
export const SCHEDULING_THRESHOLD_WINDOWS: Record<string, string[]> = {
  openai: ['5h', '7d'],
  anthropic: ['5h', '7d', '7d_oi'],
  grok: ['quota'],
  kimi: ['5h', 'weekly'],
  zhipu: ['5h', 'weekly'],
  minimax: ['5h', 'weekly'],
  opencode_go: ['5h', 'weekly', 'monthly'],
  command_code: ['5h', 'weekly', 'monthly']
}

export function isValidSchedulingThresholdWindowRule(rule: SchedulingThresholdWindowRule): boolean {
  if (rule.mode === 'fixed') return isIntegerIn(rule.threshold, 1, 100)
  if (rule.mode === 'random') {
    return isValidSchedulingThresholdRange({
      min: rule.min ?? Number.NaN,
      max: rule.max ?? Number.NaN,
      full_monthly_limit: rule.full_monthly_limit ?? 0
    })
  }
  return false
}

export function isValidSchedulingThresholdWindowRules(rules: SchedulingThresholdWindowRules): boolean {
  return Object.values(rules).every(isValidSchedulingThresholdWindowRule)
}

// Reads stored rules, keeping only valid rules for the platform's windows.
export function parseSchedulingThresholdWindowRules(raw: unknown, platform: string | undefined): SchedulingThresholdWindowRules {
  const rules: SchedulingThresholdWindowRules = {}
  if (!raw || typeof raw !== 'object') return rules
  for (const window of SCHEDULING_THRESHOLD_WINDOWS[platform ?? ''] ?? []) {
    const value = (raw as Record<string, unknown>)[window]
    if (!value || typeof value !== 'object') continue
    const source = value as Record<string, unknown>
    const rule: SchedulingThresholdWindowRule =
      source.mode === 'fixed'
        ? { mode: 'fixed', threshold: Number(source.threshold) }
        : {
            mode: 'random',
            min: Number(source.min),
            max: Number(source.max),
            full_monthly_limit: Number(source.full_monthly_limit ?? 0)
          }
    if ((source.mode === 'fixed' || source.mode === 'random') && isValidSchedulingThresholdWindowRule(rule)) {
      rules[window] = rule
    }
  }
  return rules
}
