import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountPoolReadOnlyView from '../AccountPoolReadOnlyView.vue'
const { list, stats } = vi.hoisted(() => ({ list: vi.fn(), stats: vi.fn() }))
vi.mock('@/api/accountViewer', () => ({ accountViewerAPI: { list, stats } }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

const account = (id: number) => ({ id, name: `*** #${id}`, platform: 'anthropic', type: 'oauth', status: 'active', schedulable: true, groups: [{ id: 10, name: 'Allowed pool' }], usage_windows: [] })
const result = (tokens: number) => ({ summary: { days: 30, requests: 3, tokens, cost: 0.5 }, history: [] })

describe('AccountPoolReadOnlyView', () => {
  beforeEach(() => {
    list.mockReset(); stats.mockReset()
    list.mockResolvedValue({ items: [account(1), account(2)], total: 2, pages: 1 })
    stats.mockResolvedValue(result(42))
  })
  it('loads only the read-only surface and displays numeric statistics', async () => {
    const wrapper = mount(AccountPoolReadOnlyView)
    await flushPromises()
    expect(list).toHaveBeenCalledWith({ page: 1, page_size: 20 }, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('*** #1')
    expect(wrapper.text()).toContain('Allowed pool')
    expect(wrapper.findAll('input')).toHaveLength(0)
    const buttons = wrapper.findAll('button')
    expect(buttons.every(button => ['accountViewer.refresh', 'accountViewer.stats', 'accountViewer.previous', 'accountViewer.next'].includes(button.text()))).toBe(true)
    await buttons.find(button => button.text() === 'accountViewer.stats')!.trigger('click')
    await flushPromises()
    expect(stats).toHaveBeenCalledWith(1, 30, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('0.5000')
    wrapper.unmount()
  })
  it('discards an old account response after switching the selected account', async () => {
    let finishOld!: (value: ReturnType<typeof result>) => void
    stats.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve })).mockResolvedValueOnce(result(100))
    const wrapper = mount(AccountPoolReadOnlyView)
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(button => button.text() === 'accountViewer.stats')
    await buttons[0].trigger('click')
    const firstSignal = stats.mock.calls[0][2] as AbortSignal
    await buttons[1].trigger('click')
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    finishOld(result(999))
    await flushPromises()
    expect(wrapper.text()).toContain('100')
    expect(wrapper.text()).not.toContain('999')
    wrapper.unmount()
  })
  it('clears prior account data on a failed refresh', async () => {
    const wrapper = mount(AccountPoolReadOnlyView)
    await flushPromises()
    list.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).not.toContain('*** #1')
    expect(wrapper.get('[role="alert"]').text()).toBe('accountViewer.error')
    wrapper.unmount()
  })
})
