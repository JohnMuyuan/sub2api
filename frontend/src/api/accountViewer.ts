import { apiClient } from './client'

export interface ViewerWindow { window: string; label: string; used_percent: number | null; status: string; observed_at?: string; resets_at?: string }
export interface ViewerAccount {
  id: number; name: string; platform: string; type: string; status: string; schedulable: boolean
  rate_limit_reset_at?: string; overload_until?: string; temp_unschedulable_until?: string
  groups: { id: number; name: string }[]; usage_windows: ViewerWindow[]
}
export interface ViewerStats {
  summary: { days: number; requests: number; tokens: number; cost: number }
  history: { date: string; requests: number; tokens: number; cost: number }[]
}
export const accountViewerAPI = {
  async list(params: { page: number; page_size: number }, signal?: AbortSignal) {
    const { data } = await apiClient.get<{ items: ViewerAccount[]; total: number; pages: number }>('/admin/account-viewer/accounts', { params, signal })
    return data
  },
  async stats(id: number, days: number, signal?: AbortSignal) {
    const { data } = await apiClient.get<ViewerStats>(`/admin/account-viewer/accounts/${id}/stats`, { params: { days }, signal })
    return data
  }
}
