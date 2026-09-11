import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface RequestRecord {
  id: number
  request_id?: string
  client_request_id?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  method: string
  path: string
  query_string?: string
  request_headers: Record<string, string[]>
  request_body?: string
  conversation_system?: string
  conversation_request?: string
  conversation_response?: string
  request_content_type?: string
  response_status: number
  response_headers: Record<string, string[]>
  response_body?: string
  response_content_type?: string
  stream: boolean
  user_agent?: string
  ip_address?: string
  duration_ms: number
  created_at: string
  user?: RequestRecordUser
  api_key?: RequestRecordAPIKey
  account?: RequestRecordReference
  group?: RequestRecordReference
}

export interface RequestRecordUser {
  id: number
  email: string
  username?: string
  deleted_at?: string
}

export interface RequestRecordAPIKey {
  id: number
  name: string
  user_id: number
}

export interface RequestRecordReference {
  id: number
  name: string
}

export interface RequestRecordQuery {
  page?: number
  page_size?: number
  include_payload?: boolean
  conversation_only?: boolean
  request_id?: string
  method?: string
  path?: string
  model?: string
  status_code?: number
  user_id?: number
  api_key_id?: number
  group_id?: number
  start_time?: string
  end_time?: string
}

export async function list(params: RequestRecordQuery = {}): Promise<PaginatedResponse<RequestRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<RequestRecord>>('/admin/request-records', { params })
  return data
}

export async function get(id: number): Promise<RequestRecord> {
  const { data } = await apiClient.get<RequestRecord>(`/admin/request-records/${id}`)
  return data
}

export async function exportCSV(params: Omit<RequestRecordQuery, 'page' | 'page_size'> = {}): Promise<Blob> {
  const { data } = await apiClient.get<Blob>('/admin/request-records/export', {
    params,
    responseType: 'blob'
  })
  return data
}

const requestRecordsAPI = { list, get, exportCSV }

export default requestRecordsAPI
