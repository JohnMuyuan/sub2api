import { apiClient } from '@/api/client'
import type { SchedulingThresholdWindowRules } from '@/utils/schedulingThresholdRange'

export interface AccountSchedulingThresholdRange {
  enabled: boolean
  min: number
  max: number
  full_monthly_limit: number
}

export type AccountSchedulingThresholdRanges = Record<string, AccountSchedulingThresholdRange>

// Platform -> window -> rule.
export type AccountSchedulingThresholdWindowRuleMap = Record<string, SchedulingThresholdWindowRules>

export interface AccountSchedulingThresholdCustomSettings {
  ranges: AccountSchedulingThresholdRanges
  window_rules: AccountSchedulingThresholdWindowRuleMap
}

const ENDPOINT = '/admin/settings/account-scheduling-threshold-ranges'

function normalize(data: Partial<AccountSchedulingThresholdCustomSettings>): AccountSchedulingThresholdCustomSettings {
  return { ranges: data.ranges ?? {}, window_rules: data.window_rules ?? {} }
}

export const accountSchedulingThresholdRangesAPI = {
  async get(): Promise<AccountSchedulingThresholdCustomSettings> {
    const { data } = await apiClient.get<Partial<AccountSchedulingThresholdCustomSettings>>(ENDPOINT)
    return normalize(data)
  },
  async save(settings: AccountSchedulingThresholdCustomSettings): Promise<AccountSchedulingThresholdCustomSettings> {
    const { data } = await apiClient.put<Partial<AccountSchedulingThresholdCustomSettings>>(ENDPOINT, settings)
    return normalize(data)
  }
}
