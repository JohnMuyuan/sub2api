import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountWindowPrimeSettings from './AccountWindowPrimeSettings.vue'
import { accountWindowPrimeAPI, type AccountWindowPrimeSettings as Settings } from '@/api/admin/accountWindowPrime'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin/accountWindowPrime', () => ({ accountWindowPrimeAPI: { get: vi.fn(), save: vi.fn() } }))

const sel = (id: string) => `[data-testid="${id}"]`
const stored: Settings = {
  platforms: { openai: { enabled: true, model: '' }, anthropic: { enabled: false, model: 'claude-x' } },
  delay_min_minutes: 1,
  delay_max_minutes: 5,
  active_hours_enabled: false,
  active_hours_start: '08:00',
  active_hours_end: '24:00',
  prompts: ['Hi', '早上好'],
  default_models: { openai: 'gpt-5.4-mini', anthropic: 'claude-haiku-4-5-20251001' }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(accountWindowPrimeAPI.get).mockResolvedValue(structuredClone(stored))
  vi.mocked(accountWindowPrimeAPI.save).mockImplementation(async (settings) => settings)
})

describe('AccountWindowPrimeSettings', () => {
  it('loads settings and saves edits without the default model map', async () => {
    const wrapper = mount(AccountWindowPrimeSettings)
    await flushPromises()
    expect(wrapper.get(sel('window-prime-openai-enabled')).attributes('aria-checked')).toBe('true')
    expect((wrapper.get(sel('window-prime-prompts')).element as HTMLTextAreaElement).value).toBe('Hi\n早上好')

    await wrapper.get(sel('window-prime-anthropic-enabled')).trigger('click')
    await wrapper.get(sel('window-prime-delay-max')).setValue('10')
    await wrapper.get(sel('window-prime-active-hours-enabled')).trigger('click')
    await wrapper.get(sel('window-prime-active-hours-start')).setValue('09:30')
    await wrapper.get(sel('window-prime-prompts')).setValue('one\n\n two ')
    await wrapper.get(sel('window-prime-save')).trigger('click')
    await flushPromises()

    const payload = vi.mocked(accountWindowPrimeAPI.save).mock.calls[0]?.[0]
    expect(payload).toMatchObject({
      platforms: { openai: { enabled: true, model: '' }, anthropic: { enabled: true, model: 'claude-x' } },
      delay_min_minutes: 1,
      delay_max_minutes: 10,
      active_hours_enabled: true,
      active_hours_start: '09:30',
      prompts: ['one', 'two']
    })
    expect(payload).not.toHaveProperty('default_models')
    expect(wrapper.text()).toContain('admin.settings.windowPrime.saved')
  })

  it.each([
    ['window-prime-delay-min', '9'],
    ['window-prime-delay-max', '121']
  ])('rejects an invalid delay from %s', async (id, value) => {
    const wrapper = mount(AccountWindowPrimeSettings)
    await flushPromises()
    await wrapper.get(sel(id)).setValue(value)
    await wrapper.get(sel('window-prime-save')).trigger('click')
    expect(accountWindowPrimeAPI.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.settings.windowPrime.invalid')
  })

  it('rejects a malformed active hours clock', async () => {
    const wrapper = mount(AccountWindowPrimeSettings)
    await flushPromises()
    await wrapper.get(sel('window-prime-active-hours-enabled')).trigger('click')
    await wrapper.get(sel('window-prime-active-hours-end')).setValue('25:00')
    await wrapper.get(sel('window-prime-save')).trigger('click')
    expect(accountWindowPrimeAPI.save).not.toHaveBeenCalled()
  })

  it('offers a retry when loading fails', async () => {
    vi.mocked(accountWindowPrimeAPI.get).mockRejectedValueOnce(new Error('down'))
    const wrapper = mount(AccountWindowPrimeSettings)
    await flushPromises()
    expect(wrapper.text()).toContain('admin.settings.windowPrime.loadError')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find(sel('window-prime-save')).exists()).toBe(true)
  })
})
