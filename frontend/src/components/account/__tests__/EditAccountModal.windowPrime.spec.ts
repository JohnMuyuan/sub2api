import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { Account } from '@/types'

enableAutoUnmount(afterEach)

const { updateAccountMock, showErrorMock, getAvailableModelsMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  showErrorMock: vi.fn(),
  getAvailableModelsMock: vi.fn()
}))

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
vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn(),
  getAvailableModels: getAvailableModelsMock
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildAccount(extra: Record<string, unknown> = {}, platform: Account['platform'] = 'openai', type: Account['type'] = 'oauth') {
  return {
    id: 71,
    name: 'Window prime fixture',
    notes: '',
    platform,
    type,
    credentials: { expires_at: '2027-01-01T00:00:00Z' },
    credentials_status: { has_access_token: true, has_refresh_token: true },
    extra,
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

describe('EditAccountModal window priming', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    showErrorMock.mockReset()
    updateAccountMock.mockImplementation(async (_id, payload) => ({ ...buildAccount(), ...payload }))
    getAvailableModelsMock.mockReset()
    getAvailableModelsMock.mockResolvedValue([{ id: 'gpt-5.4-mini' }, { id: 'gpt-5.4-nano' }])
  })

  it('saves an account override with model and prompts', async () => {
    const wrapper = mountModal(buildAccount({ custom_setting: 'keep-me' }))
    await flushPromises()
    expect(getAvailableModelsMock).toHaveBeenCalledWith(71)
    await wrapper.get(sel('account-window-prime-mode-on')).setValue(true)
    await wrapper.get(sel('account-window-prime-model')).setValue('gpt-5.4-nano')
    await wrapper.get(sel('account-window-prime-prompts')).setValue('早\n\n  hello ')

    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].extra).toMatchObject({
      custom_setting: 'keep-me',
      window_prime: { mode: 'on', model: 'gpt-5.4-nano', prompts: ['早', 'hello'] }
    })
  })

  it('loads a stored override and drops it when set back to follow the platform', async () => {
    const wrapper = mountModal(buildAccount({ window_prime: { mode: 'off', model: '', prompts: [] } }))
    expect((wrapper.get(sel('account-window-prime-mode-off')).element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.find(sel('account-window-prime-model')).exists()).toBe(false)

    await wrapper.get(sel('account-window-prime-mode-inherit')).setValue(true)
    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].extra).not.toHaveProperty('window_prime')
  })

  it('does not add the setting to an untouched account', async () => {
    const wrapper = mountModal(buildAccount({ custom_setting: 'keep-me' }, 'anthropic', 'setup-token'))
    expect(wrapper.find(sel('account-window-prime-section')).exists()).toBe(true)
    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].extra).not.toHaveProperty('window_prime')
  })

  it('hides the section for unsupported accounts', () => {
    expect(mountModal(buildAccount({}, 'openai', 'apikey')).find(sel('account-window-prime-section')).exists()).toBe(false)
    expect(mountModal(buildAccount({}, 'grok', 'oauth')).find(sel('account-window-prime-section')).exists()).toBe(false)
  })

  it('blocks saving too long a prompt', async () => {
    const wrapper = mountModal()
    await wrapper.get(sel('account-window-prime-mode-on')).setValue(true)
    await wrapper.get(sel('account-window-prime-prompts')).setValue('x'.repeat(201))
    await submit(wrapper)
    expect(showErrorMock).toHaveBeenCalledWith('admin.settings.windowPrime.invalid')
    expect(updateAccountMock).not.toHaveBeenCalled()
  })
})

describe('EditAccountModal window priming model picker', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    updateAccountMock.mockImplementation(async (_id, payload) => ({ ...buildAccount(), ...payload }))
    getAvailableModelsMock.mockReset()
  })

  it('accepts a custom model id', async () => {
    getAvailableModelsMock.mockResolvedValue([{ id: 'gpt-5.4-mini' }])
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get(sel('account-window-prime-mode-on')).setValue(true)
    await wrapper.get(sel('account-window-prime-model')).setValue('__custom__')
    await wrapper.get(sel('account-window-prime-model-custom')).setValue('my-model')
    await submit(wrapper)
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1].extra.window_prime.model).toBe('my-model')
  })

  it('shows a saved model that is not in the list as custom', async () => {
    getAvailableModelsMock.mockResolvedValue([{ id: 'gpt-5.4-mini' }])
    const wrapper = mountModal(buildAccount({ window_prime: { mode: 'on', model: 'old-model', prompts: [] } }))
    await flushPromises()
    expect((wrapper.get(sel('account-window-prime-model')).element as HTMLSelectElement).value).toBe('__custom__')
    expect((wrapper.get(sel('account-window-prime-model-custom')).element as HTMLInputElement).value).toBe('old-model')
  })

  it('still allows typing a model when the list cannot load', async () => {
    getAvailableModelsMock.mockRejectedValue(new Error('upstream down'))
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get(sel('account-window-prime-mode-on')).setValue(true)
    expect(wrapper.text()).toContain('admin.settings.windowPrime.modelLoadFailed')
  })
})
