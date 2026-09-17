import { httpDelete, httpGet, httpPost } from './request'
import type { PageData } from './types'

export interface TagItem {
  id: number
  name: string
  color: string
  created_at: string
  updated_at: string
}

/** 我的标签列表 */
export function listTags() {
  return httpGet<TagItem[]>('/tags/list')
}

/** 创建标签 */
export function createTag(data: { name: string; color?: string }) {
  return httpPost<TagItem>('/tags/create', data)
}

/** 删除标签 */
export function deleteTag(tagId: number) {
  return httpDelete<{ deleted: boolean }>(`/tags/${tagId}`)
}

/** 获取文件的标签 */
export function getFileTags(fileId: number) {
  return httpGet<TagItem[]>(`/tags/file/${fileId}`)
}

/** 设置文件标签（覆盖式） */
export function setFileTags(data: { file_id: number; tag_ids: number[] }) {
  return httpPost<{ ok: boolean }>('/tags/file/set', data)
}

/** 搜索标签名称 */
export function searchTags(keyword: string) {
  return httpGet<TagItem[]>('/tags/search', { q: keyword })
}

/** 获取可下载文件列表（供批量下载多选） */
export function listDownloadableFiles() {
  return httpGet<{ id: number; name: string; size: number; hash: string; parent_id: number }[]>('/files/download/files')
}
