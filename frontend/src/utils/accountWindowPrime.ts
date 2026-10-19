// Limits mirror backend/internal/service/account_window_prime.go.
export const WINDOW_PRIME_PLATFORMS = ['openai', 'anthropic'] as const
export const MAX_WINDOW_PRIME_PROMPTS = 50
export const MAX_WINDOW_PRIME_PROMPT_LENGTH = 200
export const MAX_WINDOW_PRIME_DELAY_MINUTES = 120

export type WindowPrimeMode = 'inherit' | 'on' | 'off'

export interface AccountWindowPrimeConfig {
  mode: WindowPrimeMode
  model: string
  prompts: string[]
}

export function splitWindowPrimePrompts(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
}

export function isValidWindowPrimePrompts(prompts: string[]): boolean {
  return (
    prompts.length <= MAX_WINDOW_PRIME_PROMPTS &&
    prompts.every((prompt) => Array.from(prompt).length <= MAX_WINDOW_PRIME_PROMPT_LENGTH)
  )
}

const CLOCK = /^(?:[01]\d|2[0-3]):[0-5]\d$/

export function isValidWindowPrimeClock(value: string, allowEndOfDay = false): boolean {
  const trimmed = value.trim()
  return CLOCK.test(trimmed) || (allowEndOfDay && trimmed === '24:00')
}

export function isValidWindowPrimeDelay(min: number, max: number): boolean {
  return Number.isInteger(min) && Number.isInteger(max) && min >= 0 && min <= max && max <= MAX_WINDOW_PRIME_DELAY_MINUTES
}

export function readAccountWindowPrimeConfig(extra: Record<string, unknown> | undefined): AccountWindowPrimeConfig {
  const raw = extra?.window_prime
  const config: AccountWindowPrimeConfig = { mode: 'inherit', model: '', prompts: [] }
  if (!raw || typeof raw !== 'object') return config
  const source = raw as Record<string, unknown>
  if (source.mode === 'on' || source.mode === 'off') config.mode = source.mode
  if (typeof source.model === 'string') config.model = source.model.trim()
  if (Array.isArray(source.prompts)) {
    config.prompts = source.prompts.filter((item): item is string => typeof item === 'string' && item.trim() !== '')
  }
  return config
}

// Writes the override into extra, removing it when it only follows the platform.
export function applyAccountWindowPrimeConfig(extra: Record<string, unknown>, config: AccountWindowPrimeConfig) {
  const model = config.model.trim()
  if (config.mode === 'inherit' && model === '' && config.prompts.length === 0) {
    delete extra.window_prime
    return
  }
  extra.window_prime = { mode: config.mode, model, prompts: [...config.prompts] }
}

export function supportsAccountWindowPrime(account: { platform?: string; type?: string; parent_account_id?: number | null } | null | undefined): boolean {
  if (!account || account.parent_account_id) return false
  if (account.platform === 'openai') return account.type === 'oauth'
  if (account.platform === 'anthropic') return account.type === 'oauth' || account.type === 'setup-token'
  return false
}
