import { apiClient } from '@/api/client'

export interface SystemTimezoneSettings {
  timezone: string
  effective_timezone: string
  fallback_timezone: string
  restart_required: boolean
}
export const systemTimezoneAPI = {
  async get(): Promise<SystemTimezoneSettings> {
    return (await apiClient.get<SystemTimezoneSettings>('/admin/settings/system-timezone')).data
  },
  async save(timezone: string): Promise<SystemTimezoneSettings> {
    return (await apiClient.put<SystemTimezoneSettings>('/admin/settings/system-timezone', { timezone })).data
  }
}
