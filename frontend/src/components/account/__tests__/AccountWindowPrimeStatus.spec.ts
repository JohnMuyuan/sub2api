import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountWindowPrimeStatus from '../AccountWindowPrimeStatus.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key) })
}))

const now = new Date('2026-10-19T12:00:00Z')
const state = (overrides: Record<string, unknown> = {}) => ({
  window_prime_state: {
    status: 'succeeded',
    windows: ['5h', '7d'],
    attempts: 1,
    last_attempt_at: '2026-10-19T11:00:00Z',
    ...overrides
  }
})

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(now) })
afterEach(() => { vi.useRealTimers() })

describe('AccountWindowPrimeStatus', () => {
  it('shows a recent success', () => {
    const text = mount(AccountWindowPrimeStatus, { props: { extra: state() } }).text()
    expect(text).toContain('status.succeeded')
    expect(text).toContain('"windows":"5h·7d"')
  })

  it('shows a failure with the error as tooltip', () => {
    const wrapper = mount(AccountWindowPrimeStatus, { props: { extra: state({ status: 'failed', attempts: 2, error: 'API returned 429' }) } })
    expect(wrapper.text()).toContain('status.failed')
    expect(wrapper.text()).toContain('"attempts":2')
    expect(wrapper.get('[data-testid="account-window-prime-status"]').attributes('title')).toBe('API returned 429')
  })

  it('hides missing or stale results', () => {
    expect(mount(AccountWindowPrimeStatus, { props: { extra: {} } }).html()).not.toContain('account-window-prime-status')
    expect(mount(AccountWindowPrimeStatus, { props: { extra: state({ last_attempt_at: '2026-10-17T11:00:00Z' }) } }).html())
      .not.toContain('account-window-prime-status')
  })
})
