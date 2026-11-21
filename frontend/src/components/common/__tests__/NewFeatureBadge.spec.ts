import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it } from 'vitest'
import { NEW_FEATURE_NOTICES } from '@/constants/newFeatureNotices'
import NewFeatureBadge from '../NewFeatureBadge.vue'
import zhCommon from '@/i18n/locales/zh/common'

describe('NewFeatureBadge', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  function mountBadge(noticeId = NEW_FEATURE_NOTICES.protocolFeatureSwitches, userId = 41, compact = false) {
    const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh: zhCommon } })
    return mount(NewFeatureBadge, {
      props: { noticeId, userId, compact },
      global: { plugins: [i18n, createPinia()] },
    })
  }

  it('renders nothing for standard and compact badges', () => {
    const standard = mountBadge(NEW_FEATURE_NOTICES.openaiVideos, 41, false)
    expect(standard.find('[data-testid="new-feature-badge"]').exists()).toBe(false)
    expect(standard.find('button').exists()).toBe(false)
    expect(standard.text()).toBe('')
    standard.unmount()

    const compact = mountBadge(NEW_FEATURE_NOTICES.protocolFeatureSwitches, 41, true)
    expect(compact.find('[data-testid="new-feature-badge"]').exists()).toBe(false)
    expect(compact.find('button').exists()).toBe(false)
    expect(compact.text()).toBe('')
    compact.unmount()
  })
})
