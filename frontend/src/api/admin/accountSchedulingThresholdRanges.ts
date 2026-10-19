import { apiClient } from '@/api/client'

export interface AccountSchedulingThresholdRange {
  enabled: boolean
  min: number
  max: number
  full_monthly_limit: number
}

export type AccountSchedulingThresholdRanges = Record<string, AccountSchedulingThresholdRange>

export const accountSchedulingThresholdRangesAPI = {
  async get(): Promise<AccountSchedulingThresholdRanges> {
    const { data } = await apiClient.get<{ ranges: AccountSchedulingThresholdRanges }>(
      '/admin/settings/account-scheduling-threshold-ranges'
    )
    return data.ranges ?? {}
  },
  async save(ranges: AccountSchedulingThresholdRanges): Promise<AccountSchedulingThresholdRanges> {
    const { data } = await apiClient.put<{ ranges: AccountSchedulingThresholdRanges }>(
      '/admin/settings/account-scheduling-threshold-ranges',
      { ranges }
    )
    return data.ranges ?? {}
  }
}
