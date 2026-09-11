import type { AxiosResponse } from 'axios'
import request from '@/api/request'
import { message } from './notify'

async function fetchBlob(url: string, params: object): Promise<Blob> {
  const res = await request.get<Blob, AxiosResponse<Blob>>(url, { params, responseType: 'blob' })
  return res.data
}

function saveBlob(blob: Blob, filename: string) {
  if (blob.type.includes('json')) {
    blob.text().then((text) => {
      try {
        const err = JSON.parse(text)
        if (err?.message) message.error(err.message)
      } catch {
        /* ignore */
      }
    })
    return
  }
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // L11 修复：延迟释放 objectURL，避免 Firefox 等在下载启动前回收导致下载失败
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export async function downloadFile(fileId: number, filename: string) {
  const blob = await fetchBlob('/files/download', { file_id: fileId })
  saveBlob(blob, filename)
}

export async function downloadShared(token: string, fileId: number, filename: string) {
  const blob = await fetchBlob(`/share/${token}/download`, { file_id: fileId })
  saveBlob(blob, filename)
}

export async function downloadSharedFile(fileId: number, filename: string) {
  const blob = await fetchBlob('/files/shared/download', { file_id: fileId })
  saveBlob(blob, filename)
}
