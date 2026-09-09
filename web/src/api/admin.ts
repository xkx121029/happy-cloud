import { httpDelete, httpGet, httpPatch } from './request'
import type { AdminFileItem, AdminLog, AdminStats, PageData, UserInfo } from './types'

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

export function listLogs(params: { page: number; page_size: number }) {
  return httpGet<PageData<AdminLog>>('/admin/logs', params)
}
