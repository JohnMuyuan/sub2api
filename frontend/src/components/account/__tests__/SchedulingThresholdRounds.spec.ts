import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SchedulingThresholdRounds from '../SchedulingThresholdRounds.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
    te: (key: string) => key.endsWith('.5h')
  })
}))

const round = (window: string, threshold: number, limit = 0) => ({
  window,
  threshold_percent: threshold,
  used_percent: 40,
  resets_at: '2026-10-19T17:00:00Z',
  range_min: 90,
  range_max: 100,
  full_monthly_limit: limit,
  full_draws_used: 1
})

describe('SchedulingThresholdRounds', () => {
  it('renders nothing without rounds', () => {
    expect(mount(SchedulingThresholdRounds).find('[data-testid="scheduling-threshold-rounds"]').exists()).toBe(false)
  })

  it('shows the drawn pause point and a no-pause round', () => {
    const wrapper = mount(SchedulingThresholdRounds, { props: { rounds: [round('5h', 94), round('7d', 100, 2)] } })
    const chips = wrapper.findAll('[data-testid="scheduling-threshold-round"]')
    expect(chips).toHaveLength(2)
    expect(chips[0].text()).toContain('rounds.stopAt')
    expect(chips[0].text()).toContain('"percent":94')
    expect(chips[0].attributes('title')).not.toContain('fullDraws')
    expect(chips[1].text()).toContain('rounds.noStop')
    expect(chips[1].text()).toContain('"window":"7d"')
    expect(chips[1].attributes('title')).toContain('"used":1,"limit":2')
  })
})

describe('SchedulingThresholdRounds fixed windows', () => {
  it('uses the fixed tooltip for fixed windows', () => {
    const wrapper = mount(SchedulingThresholdRounds, { props: { rounds: [{ ...round('7d', 90), mode: 'fixed' as const }] } })
    expect(wrapper.get('[data-testid="scheduling-threshold-round"]').attributes('title')).toContain('rounds.fixedTooltip')
  })
})
