import { apiClient } from '@/api/client'

export interface AccountWindowPrimePlatformSettings {
  enabled: boolean
  model: string
}

export interface AccountWindowPrimeSettings {
  platforms: Record<string, AccountWindowPrimePlatformSettings>
  delay_min_minutes: number
  delay_max_minutes: number
  active_hours_enabled: boolean
  active_hours_start: string
  active_hours_end: string
  prompts: string[]
  // Returned by the server for placeholders; ignored on save.
  default_models?: Record<string, string>
}

export const accountWindowPrimeAPI = {
  async get(): Promise<AccountWindowPrimeSettings> {
    return (await apiClient.get<AccountWindowPrimeSettings>('/admin/settings/account-window-prime')).data
  },
  async save(settings: AccountWindowPrimeSettings): Promise<AccountWindowPrimeSettings> {
    return (await apiClient.put<AccountWindowPrimeSettings>('/admin/settings/account-window-prime', settings)).data
  }
}
