import type { AxiosResponse } from 'axios'
import request, { httpDelete, httpGet, httpPost, httpPut } from './request'
import type { MyShareItem, PageData, ShareInfo } from './types'

export interface CreateShareData {
  file_id: number
  password?: string
  expire_at?: string
}

export function createShare(data: CreateShareData) {
  return httpPost<ShareInfo>('/share/create', data)
}

export function getShare(token: string) {
  return httpGet<ShareInfo>(`/share/${token}`)
}

export function verifyShare(token: string, password: string) {
  return httpPost<unknown>(`/share/${token}/verify`, { password })
}

export async function downloadSharedFile(token: string, fileId: number): Promise<Blob> {
  const res = await request.get<Blob, AxiosResponse<Blob>>(`/share/${token}/download`, {
    params: { file_id: fileId },
    responseType: 'blob'
  })
  return res.data
}

/** 我的分享列表 */
export function listMyShares(page = 1, pageSize = 50) {
  return httpGet<PageData<MyShareItem>>('/share/mine/list', { page, page_size: pageSize })
}

/** 取消分享 */
export function cancelShare(id: number) {
  return httpDelete<{ cancelled: boolean }>(`/share/mine/${id}`)
}

/** 更新分享：password 传空串清除密码；expire_at 传具体时间设置有效期；
 *  clear_expire=true 表示设为永久（M1 修复：区分"未传"与"显式 null"） */
export function updateShare(id: number, data: { password?: string; expire_at?: string | null; clear_expire?: boolean }) {
  return httpPut<{ updated: boolean }>(`/share/mine/${id}`, data)
}
