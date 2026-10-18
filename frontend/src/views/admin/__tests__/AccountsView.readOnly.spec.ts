import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AccountsView from '../AccountsView.vue'

const { list, getById, getAllProxies, auth } = vi.hoisted(() => ({ list: vi.fn(), getById: vi.fn(), getAllProxies: vi.fn(), auth: { token: 'test-token', isAdmin: false, isAccountViewer: true, isSimpleMode: false } }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  accounts: {
    getManagementCapabilities: vi.fn().mockResolvedValue({ concurrency_upgrade_enabled: false }), list, getById,
    getBatchTodayStats: vi.fn().mockResolvedValue({ stats: {} }),
    getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 }),
    getUpstreamBillingRatesWithEtag: vi.fn().mockResolvedValue({ notModified: true, data: null })
  }, proxies: { getAll: getAllProxies }, groups: { getAll: vi.fn().mockResolvedValue([]) }
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const row = { id: 7, name: 'Pool ***', platform: 'openai', type: 'apikey', status: 'active', schedulable: true, concurrency: 1, priority: 3, group_ids: [], credentials: {}, extra: {} }
let wrapper: VueWrapper | undefined
const open = async () => {
  wrapper = mount(AccountsView, { global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' },
    TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
    DataTable: { props: ['data', 'columns'], template: '<div><span data-testid="cols">{{ columns.map((c) => c.key).join(",") }}</span><div v-for="row in data" :key="row.id"><slot name="cell-schedulable" :row="row" /><slot name="cell-priority" :row="row" /><slot name="cell-upstream_billing_rate" :row="row" /><slot name="cell-actions" :row="row" /></div></div>' },
    AccountTableActions: { props: ['canCreate'], template: '<div data-testid="actions" :data-can-create="String(canCreate)" />' },
    AccountTableFilters: { props: ['hideSearch'], template: '<div data-testid="filters" :data-hide-search="String(hideSearch)" />' },
    UpstreamBillingRateCell: { props: ['canProbe'], template: '<div data-testid="upstream" :data-can-probe="String(canProbe)" />' },
    AccountPriorityCell: { template: '<div data-testid="priority-editor" />' },
    AccountBulkActionsBar: { template: '<div data-testid="bulk-bar" />' }, Pagination: true,
    ConfirmDialog: true, AccountActionMenu: true, ImportDataModal: true, ReAuthAccountModal: true,
    AccountTestModal: { props: ['show', 'account'], template: '<div v-if="show" data-testid="test-modal">{{ account.id }}</div>' },
    AccountStatsModal: { props: ['show', 'account'], template: '<div v-if="show" data-testid="stats-modal">{{ account.id }}</div>' },
    IQTestModal: true, ScheduledTestsPanel: true, NewAPIUpstreamConfigDialog: true,
    SyncFromCrsModal: true, TempUnschedStatusModal: true, ErrorPassthroughRulesModal: true,
    TLSFingerprintProfilesModal: true, CreateAccountModal: true, EditAccountModal: true,
    BulkEditAccountModal: true, TotpStepUpDialog: true, Teleport: true
  } } })
  await flushPromises()
  return wrapper
}
beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  getAllProxies.mockResolvedValue([])
  list.mockResolvedValue({ items: [row], total: 1, page: 1, page_size: 20, pages: 1 })
})
afterEach(() => wrapper?.unmount())

describe('account page for the read-only account role', () => {
  it('hides management controls, proxy and note columns', async () => {
    const wrapper = await open()
    const cols = wrapper.get('[data-testid="cols"]').text().split(',')
    expect(cols).not.toContain('select')
    expect(cols).not.toContain('proxy')
    expect(cols).not.toContain('notes')
    expect(cols).toEqual(expect.arrayContaining(['capacity', 'usage', 'upstream_billing_rate', 'actions']))
    expect(wrapper.get('[data-testid="actions"]').attributes('data-can-create')).toBe('false')
    expect(wrapper.get('[data-testid="filters"]').attributes('data-hide-search')).toBe('true')
    expect(wrapper.get('[data-testid="upstream"]').attributes('data-can-probe')).toBe('false')
    expect(wrapper.find('[data-testid="bulk-bar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="priority-editor"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('common.edit')
    expect(wrapper.text()).not.toContain('common.delete')
    expect(wrapper.get('[data-testid="schedulable-label"]').text()).toBe('admin.accounts.schedulableEnabled')
    expect(wrapper.find('button.rounded-full').exists()).toBe(false)
    expect(getAllProxies).not.toHaveBeenCalled()
  })

  it('tests and opens statistics from the redacted list row without loading account details', async () => {
    const wrapper = await open()
    const buttons = wrapper.findAll('button').filter((b) => b.text().includes('admin.accounts.testConnection') || b.text().includes('admin.accounts.viewStats'))
    expect(buttons).toHaveLength(2)
    await buttons[0].trigger('click')
    await buttons[1].trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="test-modal"]').text()).toBe('7')
    expect(wrapper.get('[data-testid="stats-modal"]').text()).toBe('7')
    expect(getById).not.toHaveBeenCalled()
  })
})
