import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import type { Account } from '@/types'

enableAutoUnmount(afterEach)

const { updateAccountMock, showErrorMock } = vi.hoisted(() => ({ updateAccountMock: vi.fn(), showErrorMock: vi.fn() }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: showErrorMock, showSuccess: vi.fn(), showInfo: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: true }) }))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getManagementCapabilities: vi.fn().mockResolvedValue({
        web_search_enabled: false,
        account_quota_notify_enabled: false
      }),
      update: updateAccountMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildAccount(credentials: Record<string, unknown> = {}) {
  return {
    id: 61,
    name: 'Random threshold fixture',
    notes: '',
    platform: 'grok',
    type: 'oauth',
    credentials: { expires_at: '2027-01-01T00:00:00Z', token_type: 'Bearer', ...credentials },
    credentials_status: { has_access_token: true, has_refresh_token: true },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as Account
}

function mountModal(account = buildAccount()) {
  return mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true
      }
    }
  })
}

const sel = (id: string) => `[data-testid="${id}"]`

async function submit(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
}

describe('EditAccountModal random scheduling threshold', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    showErrorMock.mockReset()
    updateAccountMock.mockImplementation(async (_id, payload) => ({ ...buildAccount(), ...payload }))
  })

  it('loads a stored range and saves it back with the minimum as fixed fallback', async () => {
    const range = { min: 90, max: 100, full_monthly_limit: 2 }
    const wrapper = mountModal(buildAccount({ account_scheduling_threshold: 90, account_scheduling_threshold_range: range }))
    expect((wrapper.get(sel('account-scheduling-threshold-override-enabled')).element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get(sel('account-scheduling-threshold-range-enabled')).element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get(sel('account-scheduling-threshold-range-max')).element as HTMLInputElement).value).toBe('100')
    expect(wrapper.find(sel('account-scheduling-threshold-override-value')).exists()).toBe(false)

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].credentials).toMatchObject({
      account_scheduling_threshold: 90,
      account_scheduling_threshold_range: range
    })
  })

  it('enables a new range from the edit form', async () => {
    const wrapper = mountModal()
    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-range-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-range-min')).setValue('92')
    await wrapper.get(sel('account-scheduling-threshold-range-max')).setValue('97')
    await wrapper.get(sel('account-scheduling-threshold-range-full-monthly-limit')).setValue('0')

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].credentials).toMatchObject({
      account_scheduling_threshold: 92,
      account_scheduling_threshold_range: { min: 92, max: 97, full_monthly_limit: 0 }
    })
  })

  it('removes the range when switching back to a fixed value', async () => {
    const wrapper = mountModal(buildAccount({
      account_scheduling_threshold: 90,
      account_scheduling_threshold_range: { min: 90, max: 97, full_monthly_limit: 0 }
    }))
    await wrapper.get(sel('account-scheduling-threshold-range-enabled')).setValue(false)
    await wrapper.get(sel('account-scheduling-threshold-override-value')).setValue('85')

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const credentials = updateAccountMock.mock.calls[0]?.[1].credentials
    expect(credentials.account_scheduling_threshold).toBe(85)
    expect(credentials.account_scheduling_threshold_range).toBeNull()
  })

  it('removes both keys when the override is turned off', async () => {
    const wrapper = mountModal(buildAccount({
      account_scheduling_threshold: 90,
      account_scheduling_threshold_range: { min: 90, max: 97, full_monthly_limit: 0 }
    }))
    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(false)

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    const credentials = updateAccountMock.mock.calls[0]?.[1].credentials
    expect(credentials.account_scheduling_threshold).toBeNull()
    expect(credentials.account_scheduling_threshold_range).toBeNull()
  })

  it('blocks saving an invalid range', async () => {
    const wrapper = mountModal()
    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-range-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-range-min')).setValue('97')
    await wrapper.get(sel('account-scheduling-threshold-range-max')).setValue('90')
    expect(wrapper.find(sel('account-scheduling-threshold-range-invalid')).exists()).toBe(true)

    await submit(wrapper)
    expect(showErrorMock).toHaveBeenCalledWith('admin.settings.schedulingThresholdRange.invalidAccount')
    expect(updateAccountMock).not.toHaveBeenCalled()
  })
})

describe('EditAccountModal per-window scheduling thresholds', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    showErrorMock.mockReset()
    updateAccountMock.mockImplementation(async (_id, payload) => ({ ...buildAccount(), ...payload }))
  })

  it('saves a window rule next to the account default', async () => {
    const wrapper = mountModal()
    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-override-value')).setValue('95')
    await wrapper.get(sel('account-scheduling-threshold-window-quota-mode')).setValue('fixed')
    await wrapper.get(sel('account-scheduling-threshold-window-quota-threshold')).setValue('80')

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].credentials).toMatchObject({
      account_scheduling_threshold: 95,
      account_scheduling_threshold_windows: { quota: { mode: 'fixed', threshold: 80 } }
    })
  })

  it('loads stored window rules and removes them with the override', async () => {
    const wrapper = mountModal(buildAccount({
      account_scheduling_threshold: 90,
      account_scheduling_threshold_windows: { quota: { mode: 'random', min: 70, max: 80, full_monthly_limit: 0 } }
    }))
    expect((wrapper.get(sel('account-scheduling-threshold-window-quota-mode')).element as HTMLSelectElement).value).toBe('random')
    expect((wrapper.get(sel('account-scheduling-threshold-window-quota-min')).element as HTMLInputElement).value).toBe('70')

    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(false)
    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].credentials.account_scheduling_threshold_windows).toBeNull()
  })

  it('blocks saving an invalid window rule', async () => {
    const wrapper = mountModal()
    await wrapper.get(sel('account-scheduling-threshold-override-enabled')).setValue(true)
    await wrapper.get(sel('account-scheduling-threshold-window-quota-mode')).setValue('fixed')
    await wrapper.get(sel('account-scheduling-threshold-window-quota-threshold')).setValue('101')
    await submit(wrapper)
    expect(showErrorMock).toHaveBeenCalledWith('admin.settings.schedulingThresholdRange.windowRules.invalid')
    expect(updateAccountMock).not.toHaveBeenCalled()
  })
})
