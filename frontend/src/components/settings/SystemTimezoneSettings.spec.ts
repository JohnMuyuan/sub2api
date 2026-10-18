import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SystemTimezoneSettings from './SystemTimezoneSettings.vue'
import { systemTimezoneAPI } from '@/api/admin/systemTimezone'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/systemTimezone', () => ({ systemTimezoneAPI: { get: vi.fn(), save: vi.fn() } }))
const initial = { timezone: '', effective_timezone: 'UTC', fallback_timezone: 'UTC', restart_required: false }
beforeEach(() => { vi.clearAllMocks(); vi.mocked(systemTimezoneAPI.get).mockResolvedValue(initial) })
describe('SystemTimezoneSettings', () => {
  it('saves a chosen timezone and shows restart pending without switching the effective clock', async () => {
    vi.mocked(systemTimezoneAPI.save).mockResolvedValue({ ...initial, timezone: 'Asia/Taipei', restart_required: true })
    const wrapper = mount(SystemTimezoneSettings)
    await flushPromises()
    await wrapper.get('input').setValue('Asia/Taipei')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(systemTimezoneAPI.save).toHaveBeenCalledWith('Asia/Taipei')
    expect(wrapper.text()).toContain('admin.settings.systemTimezone.pending')
    expect(wrapper.text()).toContain('admin.settings.systemTimezone.saved')
  })
  it('keeps the existing state when save fails and supports blank fallback', async () => {
    vi.mocked(systemTimezoneAPI.save).mockRejectedValue(new Error('invalid'))
    const wrapper = mount(SystemTimezoneSettings)
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(systemTimezoneAPI.save).toHaveBeenCalledWith('')
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('admin.settings.systemTimezone.pending')
  })
})
