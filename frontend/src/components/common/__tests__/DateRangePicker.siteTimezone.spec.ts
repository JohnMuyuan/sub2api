import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DateRangePicker from '../DateRangePicker.vue'
import { setSiteTimeZone } from '@/utils/siteTimezone'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
afterEach(() => { vi.useRealTimers(); setSiteTimeZone() })
describe('site calendar presets', () => {
  it.each([
    ['dates.today', '2026-11-01', '2026-11-01'],
    ['dates.yesterday', '2026-10-31', '2026-10-31'],
    ['dates.thisMonth', '2026-11-01', '2026-11-01'],
    ['dates.lastMonth', '2026-10-01', '2026-10-31'],
    ['dates.last7Days', '2026-10-26', '2026-11-01'],
  ])('uses Taipei for %s across a browser date/month boundary', async (label, start, end) => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-31T16:30:00Z'))
    setSiteTimeZone('Asia/Taipei')
    const wrapper = mount(DateRangePicker, { props: { startDate: '', endDate: '' } })
    await wrapper.get('button').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === label)!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'dates.apply')!.trigger('click')
    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([start])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([end])
    wrapper.unmount()
  })
})
