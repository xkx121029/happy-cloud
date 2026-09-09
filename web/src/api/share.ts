import type { AxiosResponse } from 'axios'
import request, { httpGet, httpPost } from './request'
import type { ShareInfo } from './types'

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
