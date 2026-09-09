import { httpGet, httpPost, httpUpload } from './request'
import type { FileItem, PageData, QuotaInfo } from './types'

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
