import { apiClient } from '../client'
import type { IQCustomQuestion, IQCustomQuestionInput } from '@/types'

export async function listIQCustomQuestions(): Promise<IQCustomQuestion[]> {
  const { data } = await apiClient.get<IQCustomQuestion[]>('/admin/iq-custom-questions')
  return Array.isArray(data) ? data : []
}

export async function createIQCustomQuestion(input: IQCustomQuestionInput): Promise<IQCustomQuestion> {
  const { data } = await apiClient.post<IQCustomQuestion>('/admin/iq-custom-questions', input)
  return data
}

export async function updateIQCustomQuestion(id: string, input: IQCustomQuestionInput): Promise<IQCustomQuestion> {
  const { data } = await apiClient.put<IQCustomQuestion>(`/admin/iq-custom-questions/${encodeURIComponent(id)}`, input)
  return data
}

export async function deleteIQCustomQuestion(id: string): Promise<void> {
  await apiClient.delete(`/admin/iq-custom-questions/${encodeURIComponent(id)}`)
}
