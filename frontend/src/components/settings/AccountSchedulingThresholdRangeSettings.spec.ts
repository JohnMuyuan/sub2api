import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountSchedulingThresholdRangeSettings from './AccountSchedulingThresholdRangeSettings.vue'
import { accountSchedulingThresholdRangesAPI } from '@/api/admin/accountSchedulingThresholdRanges'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin/accountSchedulingThresholdRanges', () => ({
  accountSchedulingThresholdRangesAPI: { get: vi.fn(), save: vi.fn() }
}))

const sel = (id: string) => `[data-testid="${id}"]`

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(accountSchedulingThresholdRangesAPI.get).mockResolvedValue({
    ranges: {
      openai: { enabled: true, min: 90, max: 97, full_monthly_limit: 0 },
      grok: { enabled: false, min: 80, max: 85, full_monthly_limit: 1 }
    },
    window_rules: { anthropic: { '7d_oi': { mode: 'fixed', threshold: 60 } } }
  })
  vi.mocked(accountSchedulingThresholdRangesAPI.save).mockImplementation(async (settings) => settings)
})

describe('AccountSchedulingThresholdRangeSettings', () => {
  it('loads stored ranges and shows fields only for enabled platforms', async () => {
    const wrapper = mount(AccountSchedulingThresholdRangeSettings, { props: { thresholds: { anthropic: 95 } } })
    await flushPromises()
    expect(wrapper.get(sel('scheduling-threshold-range-openai-enabled')).attributes('aria-checked')).toBe('true')
    expect((wrapper.get(sel('scheduling-threshold-range-openai-min')).element as HTMLInputElement).value).toBe('90')
    expect(wrapper.find(sel('scheduling-threshold-range-grok-min')).exists()).toBe(false)
  })

  it('saves enabled ranges and keeps disabled values', async () => {
    const wrapper = mount(AccountSchedulingThresholdRangeSettings)
    await flushPromises()
    await wrapper.get(sel('scheduling-threshold-range-anthropic-enabled')).trigger('click')
    await wrapper.get(sel('scheduling-threshold-range-anthropic-max')).setValue('100')
    await wrapper.get(sel('scheduling-threshold-range-anthropic-full-monthly-limit')).setValue('2')
    await wrapper.get(sel('scheduling-threshold-range-save')).trigger('click')
    await flushPromises()

    const payload = vi.mocked(accountSchedulingThresholdRangesAPI.save).mock.calls[0]?.[0].ranges
    expect(payload?.openai).toEqual({ enabled: true, min: 90, max: 97, full_monthly_limit: 0 })
    expect(payload?.anthropic).toEqual({ enabled: true, min: 90, max: 100, full_monthly_limit: 2 })
    expect(payload?.grok).toEqual({ enabled: false, min: 80, max: 85, full_monthly_limit: 1 })
    expect(wrapper.text()).toContain('admin.settings.schedulingThresholdRange.saved')
  })

  it('refuses to save an invalid enabled range', async () => {
    const wrapper = mount(AccountSchedulingThresholdRangeSettings)
    await flushPromises()
    await wrapper.get(sel('scheduling-threshold-range-openai-min')).setValue('99')
    await wrapper.get(sel('scheduling-threshold-range-openai-max')).setValue('90')
    await wrapper.get(sel('scheduling-threshold-range-save')).trigger('click')
    await flushPromises()
    expect(accountSchedulingThresholdRangesAPI.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.settings.schedulingThresholdRange.invalid')
  })

  it('offers a retry when loading fails', async () => {
    vi.mocked(accountSchedulingThresholdRangesAPI.get).mockRejectedValueOnce(new Error('down'))
    const wrapper = mount(AccountSchedulingThresholdRangeSettings)
    await flushPromises()
    expect(wrapper.text()).toContain('admin.settings.schedulingThresholdRange.loadError')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find(sel('scheduling-threshold-range-save')).exists()).toBe(true)
  })
})

describe('AccountSchedulingThresholdRangeSettings per-window rules', () => {
  it('loads, edits and saves per-window rules', async () => {
    const wrapper = mount(AccountSchedulingThresholdRangeSettings)
    await flushPromises()
    expect((wrapper.get(sel('scheduling-threshold-window-anthropic-7d_oi-mode')).element as HTMLSelectElement).value).toBe('fixed')
    expect((wrapper.get(sel('scheduling-threshold-window-anthropic-7d_oi-threshold')).element as HTMLInputElement).value).toBe('60')
    expect(wrapper.find(sel('scheduling-threshold-window-grok-quota-mode')).exists()).toBe(true)

    await wrapper.get(sel('scheduling-threshold-window-openai-5h-mode')).setValue('fixed')
    await wrapper.get(sel('scheduling-threshold-window-openai-5h-threshold')).setValue('85')
    await wrapper.get(sel('scheduling-threshold-window-openai-7d-mode')).setValue('random')
    await wrapper.get(sel('scheduling-threshold-window-openai-7d-max')).setValue('100')
    await wrapper.get(sel('scheduling-threshold-window-openai-7d-full-monthly-limit')).setValue('2')
    await wrapper.get(sel('scheduling-threshold-window-anthropic-7d_oi-mode')).setValue('inherit')
    await wrapper.get(sel('scheduling-threshold-range-save')).trigger('click')
    await flushPromises()

    expect(vi.mocked(accountSchedulingThresholdRangesAPI.save).mock.calls[0]?.[0].window_rules).toEqual({
      openai: {
        '5h': { mode: 'fixed', threshold: 85 },
        '7d': { mode: 'random', min: 90, max: 100, full_monthly_limit: 2 }
      }
    })
  })

  it('refuses to save an invalid window threshold', async () => {
    const wrapper = mount(AccountSchedulingThresholdRangeSettings)
    await flushPromises()
    await wrapper.get(sel('scheduling-threshold-window-openai-5h-mode')).setValue('fixed')
    await wrapper.get(sel('scheduling-threshold-window-openai-5h-threshold')).setValue('0')
    await wrapper.get(sel('scheduling-threshold-range-save')).trigger('click')
    await flushPromises()
    expect(accountSchedulingThresholdRangesAPI.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.settings.schedulingThresholdRange.windowRules.invalid')
  })
})
