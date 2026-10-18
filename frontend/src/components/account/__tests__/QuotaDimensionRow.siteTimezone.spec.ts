import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import QuotaDimensionRow from '../QuotaDimensionRow.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
describe('quota timezone override', () => {
  it('preserves follow-site on switching to fixed mode and supports explicit UTC', async () => {
    const wrapper = mount(QuotaDimensionRow, { props: {
      dim: 'daily', label: 'Daily', limit: 10, resetMode: 'rolling', resetHour: null,
      quotaNotifyGlobalEnabled: false, notifyEnabled: null, notifyThreshold: null, notifyThresholdType: null,
      resetDay: null, resetTimezone: null, timezoneOptions: ['UTC', 'Asia/Taipei'],
      hourOptions: [0, 1], dayOptions: [], hintFixed: '', hintRolling: ''
    } })
    const mode = wrapper.find('select')
    await mode.setValue('fixed')
    expect(wrapper.emitted('update:resetTimezone')).toBeUndefined()
    await wrapper.setProps({ resetMode: 'fixed' })
    const zone = wrapper.findAll('select').at(-1)!
    expect((zone.element as HTMLSelectElement).value).toBe('')
    await zone.setValue('UTC')
    expect(wrapper.emitted('update:resetTimezone')?.at(-1)).toEqual(['UTC'])
    await zone.setValue('')
    expect(wrapper.emitted('update:resetTimezone')?.at(-1)).toEqual([null])
  })
})
