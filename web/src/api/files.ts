import { httpGet, httpPost, httpUpload, request } from './request'
import type { FileItem, PageData, QuotaInfo, SharedFileItem } from './types'

export interface HashCheckResult {
  exists: boolean
  file_id?: number
}

export function listFiles(parentId: number, page = 1, pageSize = 1000) {
  return httpGet<FileItem[] | PageData<FileItem>>('/files/list', {
    parent_id: parentId,
    page,
    page_size: pageSize
  })
}

/** 回收站列表 */
export function listTrash(keyword = '', page = 1, pageSize = 100) {
  return httpGet<PageData<FileItem>>('/files/trash', { keyword, page, page_size: pageSize })
}

/** 恢复回收站文件 */
export function restoreFiles(fileIds: number[]) {
  return httpPost<{ restored: number }>('/files/restore', { file_ids: fileIds })
}

/** 彻底删除（回收站内） */
export function deletePermanent(fileIds: number[]) {
  return httpPost<{ purged: number }>('/files/delete/permanent', { file_ids: fileIds })
}

/** 清空回收站 */
export function clearTrash() {
  return httpPost<{ purged: number }>('/files/trash/clear')
}

/** 全局搜索 */
export function searchFiles(keyword: string, type = -1) {
  return httpGet<FileItem[]>('/files/search', { keyword, type })
}

/** 最近文件 */
export function recentFiles(limit = 50) {
  return httpGet<FileItem[]>('/files/recent', { limit })
}

/** 我的收藏 */
export function listFavorites() {
  return httpGet<FileItem[]>('/files/favorites')
}

/** 收藏/取消收藏 */
export function favoriteFile(fileId: number) {
  return httpPost<{ favorited: boolean }>('/files/favorite', { file_id: fileId })
}

export function unfavoriteFile(fileId: number) {
  return httpPost<{ favorited: boolean }>('/files/unfavorite', { file_id: fileId })
}

/** 批量移动 */
export function batchMove(data: { file_ids: number[]; target_parent_id: number }) {
  return httpPost<{ moved: number }>('/files/batch-move', data)
}

/** 预览文件：返回 blob（需鉴权），供 object URL 使用 */
export async function fetchPreviewBlob(fileId: number): Promise<Blob> {
  const res = await request.get(`/files/preview`, {
    params: { file_id: fileId },
    responseType: 'blob'
  })
  return res.data as Blob
}

/** 存储概览（配额 + 分类占用） */
export function fetchStorageOverview() {
  return httpGet<{
    quota_max: number
    quota_used: number
    categories: { image: number; video: number; audio: number; doc: number; other: number }
  }>('/files/overview')
}

export function mkdir(data: { parent_id: number; name: string }) {
  return httpPost<FileItem>('/files/mkdir', data)
}

export function checkHash(data: { name: string; size: number; hash: string; parent_id: number }) {
  return httpPost<HashCheckResult>('/files/upload/hash', data)
}

export function uploadDirect(form: FormData, onProgress?: (percent: number) => void) {
  return httpUpload<FileItem>('/files/upload', form, onProgress)
}

export function uploadChunk(form: FormData, onProgress?: (percent: number) => void) {
  return httpUpload<unknown>('/files/upload/chunk', form, onProgress)
}

export function mergeChunks(data: {
  hash: string
  name: string
  size: number
  parent_id: number
  chunk_total: number
}) {
  return httpPost<FileItem>('/files/upload/merge', data)
}

export function renameFile(data: { file_id: number; new_name: string }) {
  return httpPost<unknown>('/files/rename', data)
}

export function moveFile(data: { file_id: number; target_parent_id: number }) {
  return httpPost<unknown>('/files/move', data)
}

export function deleteFiles(fileIds: number[]) {
  return httpPost<unknown>('/files/delete', { file_ids: fileIds })
}

export function fetchQuota() {
  return httpGet<QuotaInfo>('/files/quota')
}

/** 共享/取消共享到公共目录 */
export function toggleShared(data: { file_id: number; shared: boolean }) {
  return httpPost<{ is_shared: 0 | 1 }>('/files/shared/toggle', data)
}

/** 公共共享目录列表 */
export function listShared() {
  return httpGet<SharedFileItem[]>('/files/shared/list')
}
