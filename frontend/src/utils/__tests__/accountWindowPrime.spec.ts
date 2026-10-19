import { describe, expect, it } from 'vitest'
import {
  applyAccountWindowPrimeConfig,
  isValidWindowPrimeClock,
  isValidWindowPrimeDelay,
  isValidWindowPrimePrompts,
  readAccountWindowPrimeConfig,
  splitWindowPrimePrompts,
  supportsAccountWindowPrime
} from '../accountWindowPrime'

describe('accountWindowPrime', () => {
  it('splits prompts by line and drops blanks', () => {
    expect(splitWindowPrimePrompts(' Hi \n\n  早上好\n')).toEqual(['Hi', '早上好'])
  })

  it('validates prompts, clocks and delays like the backend', () => {
    expect(isValidWindowPrimePrompts(['a'.repeat(200)])).toBe(true)
    expect(isValidWindowPrimePrompts(['a'.repeat(201)])).toBe(false)
    expect(isValidWindowPrimePrompts(Array(51).fill('x'))).toBe(false)
    expect(isValidWindowPrimeClock('08:00')).toBe(true)
    expect(isValidWindowPrimeClock('8:00')).toBe(false)
    expect(isValidWindowPrimeClock('24:00')).toBe(false)
    expect(isValidWindowPrimeClock('24:00', true)).toBe(true)
    expect(isValidWindowPrimeDelay(1, 5)).toBe(true)
    expect(isValidWindowPrimeDelay(0, 0)).toBe(true)
    expect(isValidWindowPrimeDelay(6, 5)).toBe(false)
    expect(isValidWindowPrimeDelay(1, 121)).toBe(false)
    expect(isValidWindowPrimeDelay(1.5, 5)).toBe(false)
  })

  it('reads and writes the account override', () => {
    expect(readAccountWindowPrimeConfig(undefined)).toEqual({ mode: 'inherit', model: '', prompts: [] })
    expect(readAccountWindowPrimeConfig({ window_prime: { mode: 'on', model: ' m ', prompts: ['a', 1, ' '] } }))
      .toEqual({ mode: 'on', model: 'm', prompts: ['a'] })
    expect(readAccountWindowPrimeConfig({ window_prime: { mode: 'weird' } }).mode).toBe('inherit')

    const extra: Record<string, unknown> = { keep: true, window_prime: { mode: 'on' } }
    applyAccountWindowPrimeConfig(extra, { mode: 'inherit', model: '', prompts: [] })
    expect(extra).toEqual({ keep: true })
    applyAccountWindowPrimeConfig(extra, { mode: 'off', model: '', prompts: [] })
    expect(extra.window_prime).toEqual({ mode: 'off', model: '', prompts: [] })
  })

  it('supports only OpenAI OAuth and Anthropic OAuth or setup-token accounts', () => {
    expect(supportsAccountWindowPrime({ platform: 'openai', type: 'oauth' })).toBe(true)
    expect(supportsAccountWindowPrime({ platform: 'openai', type: 'apikey' })).toBe(false)
    expect(supportsAccountWindowPrime({ platform: 'anthropic', type: 'setup-token' })).toBe(true)
    expect(supportsAccountWindowPrime({ platform: 'grok', type: 'oauth' })).toBe(false)
    expect(supportsAccountWindowPrime({ platform: 'openai', type: 'oauth', parent_account_id: 3 })).toBe(false)
  })
})
