import { httpDelete, httpGet, httpPatch, httpPost, httpPut } from './request'
import type { AdminFileItem, AdminLog, AdminStats, DedupStats, PageData, SystemSettings, TrendData, UserInfo } from './types'

export interface BlobItem {
  hash: string
  size: number
  ref_count: number
  billed_user_id: number
  billed_username: string
  created_at: string
  on_disk: boolean
}

export interface BlobRefItem {
  file_id: number
  name: string
  user_id: number
  username: string
  size: number
  is_deleted: number
  created_at: string
}

export function listUsers(params: { page: number; page_size: number; keyword?: string }) {
  return httpGet<PageData<UserInfo>>('/admin/users', params)
}

export function updateUser(id: number, data: { status?: number; quota_max?: number }) {
  return httpPatch<unknown>(`/admin/users/${id}`, data)
}

export function deleteUser(id: number) {
  return httpDelete<unknown>(`/admin/users/${id}`)
}

export function listAdminFiles(params: { page: number; page_size: number; keyword?: string }) {
  return httpGet<PageData<AdminFileItem>>('/admin/files', params)
}

export function forceDeleteFile(id: number) {
  return httpDelete<unknown>(`/admin/files/${id}`)
}

export function fetchStats() {
  return httpGet<AdminStats>('/admin/stats')
}

export function fetchSettings() {
  return httpGet<SystemSettings>('/admin/settings')
}

export function saveSettings(data: Partial<SystemSettings>) {
  return httpPut<SystemSettings>('/admin/settings', data)
}

export function fetchTrends(days = 30) {
  return httpGet<TrendData>('/admin/trends', { days })
}

export function listLogs(params: { page: number; page_size: number }) {
  return httpGet<PageData<AdminLog>>('/admin/logs', params)
}

export function fetchStorageIndex() {
  return httpGet<DedupStats>('/admin/storage/index')
}

export function listBlobs(params: { page: number; page_size: number; keyword?: string }) {
  return httpGet<PageData<BlobItem>>('/admin/blobs', params)
}

export function fetchBlobRefs(hash: string) {
  return httpGet<{ hash: string; refs: BlobRefItem[] }>(`/admin/blobs/${encodeURIComponent(hash)}/refs`)
}

export function verifyStorage(data: { apply: boolean; gc_orphans: boolean }) {
  return httpPost<import('./types').PageData<unknown>>('/admin/storage/verify', data)
}
