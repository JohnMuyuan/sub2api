import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIRequestTimezoneField from '../OpenAIRequestTimezoneField.vue'
import Select from '@/components/common/Select.vue'
const { getTimezones } = vi.hoisted(() => ({ getTimezones: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getOpenAIRequestTimezones: getTimezones } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => { getTimezones.mockResolvedValue({ default: 'Asia/Singapore', timezones: ['Asia/Singapore', 'America/New_York'] }) })
describe('account request timezone', () => {
  it.each(['', 'auto', 'America/New_York'])('preserves generic mode %s and offers automatic egress', async (value) => {
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: value, generic: true } })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    const options = wrapper.findComponent(Select).props('options')
    expect(options).toEqual(expect.arrayContaining([
      expect.objectContaining({ value: '' }),
      expect.objectContaining({ value: 'auto' })
    ]))
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'auto')
    expect(wrapper.emitted('update:modelValue')).toEqual([['auto']])
  })
  it('clears unsupported generic settings instead of selecting a timezone', async () => {
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: 'Unknown/Zone', generic: true } })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')).toEqual([['']])
  })
  it('loads the catalog and preserves the saved timezone', async () => {
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: 'America/New_York' } })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.findComponent(Select).vm.$emit('update:modelValue', 'Asia/Singapore')
    expect(wrapper.emitted('update:modelValue')).toEqual([['Asia/Singapore']])
  })
  it('uses the server default for an unsupported saved timezone', async () => {
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: 'Unknown/Zone' } })
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')).toEqual([['Asia/Singapore']])
  })
  it('does not overwrite the saved timezone when the catalog fails', async () => {
    getTimezones.mockRejectedValue(new Error('offline'))
    const wrapper = mount(OpenAIRequestTimezoneField, { props: { modelValue: 'America/New_York' } })
    await flushPromises()
    expect(wrapper.text()).toContain('requestTimezoneLoadFailed')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
