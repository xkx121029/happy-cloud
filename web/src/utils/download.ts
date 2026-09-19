import type { AxiosResponse } from 'axios'
import request from '@/api/request'
import { message } from './notify'
import { useSettingsStore } from '@/stores/settings'
import { useUserStore } from '@/stores/user'

/** 并发分片下载：用浏览器原生 Range 头 + fetch 并行拉取，最后拼成 Blob。
 * 对大文件明显提速；小文件退化到单连接（避免小文件被并发打爆）。
 */
export async function downloadFile(fileId: number, filename: string, fileSize = 0) {
  const settings = useSettingsStore()
  if (fileSize <= 0 || settings.parallelDownload <= 1) {
    const blob = await fetchBlob('/files/download', { file_id: fileId })
    saveBlob(blob, filename)
    return
  }
  const totalParts = Math.min(settings.parallelDownload, 8)
  // 至少 4MB 才切分，避免碎片过多
  if (fileSize < 4 * 1024 * 1024) {
    const blob = await fetchBlob('/files/download', { file_id: fileId })
    saveBlob(blob, filename)
    return
  }
  const partSize = Math.ceil(fileSize / totalParts)
  const chunks: Promise<Blob>[] = []
  for (let start = 0; start < fileSize; start += partSize) {
    const end = Math.min(start + partSize - 1, fileSize - 1)
    chunks.push(fetchRange(start, end, fileId))
  }
  const blobs = await Promise.all(chunks)
  saveBlob(new Blob(blobs), filename)
}

/** 隧道内分片下载并聚合为 Blob（返回 null 表示不可用）—— 暂未接入区间帧，兜底走 HTTP */
async function p2pDownload(fileId: number, filename: string): Promise<Blob | null> {
  try {
    const { useP2P } = await import('@/p2p')
    const t = await useP2P().get()
    if (!t) return null
    const parts: BlobPart[] = []
    let received = 0
    const blob = await new Promise<Blob>((resolve, reject) => {
      const id = (Math.random() * 0xffffff) | 0
      const timer = setTimeout(() => reject(new Error('P2P 下载超时')), 120000)
      t.downloadStream(fileId, id, {
        onMeta: () => { /* 暂不展示元信息 */ },
        onData: (bin, size) => {
          parts.push(bin.buffer.slice(bin.byteOffset, bin.byteOffset + bin.byteLength) as ArrayBuffer)
          received += size
        },
        onDone: () => {
          clearTimeout(timer)
          resolve(new Blob(parts))
        },
        onError: (msg) => {
          clearTimeout(timer)
          reject(new Error(msg))
        }
      })
    })
    return blob
  } catch {
    return null
  }
}

async function fetchBlob(url: string, params: object): Promise<Blob> {
  const res = await request.get<Blob, AxiosResponse<Blob>>(url, { params, responseType: 'blob' })
  return res.data
}

/** 单 Range 下载：只下载 [start, end] 这段 */
async function fetchRange(start: number, end: number, fileId: number): Promise<Blob> {
  const url = new URL('/files/download', location.origin)
  url.searchParams.set('file_id', String(fileId))
  const res = await fetch(url.toString(), {
    headers: { Range: `bytes=${start}-${end}` }
  })
  if (!res.ok) throw new Error(`下载失败: ${res.status}`)
  return res.blob()
}

function saveBlob(blob: Blob, filename: string) {
  // 尝试使用 File System Access API（FSA），用户直接选择落盘位置；
  // 不支持的浏览器退回传统 <a> 下载
  const pick = (typeof (globalThis as any).showSaveFilePicker) !== 'undefined'
  if (pick) {
    handleFSA(blob, filename)
    return
  }
  legacySave(blob, filename)
}

/** 传统下载：createObjectURL + <a> 触发 */
function legacySave(blob: Blob, filename: string) {
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

/** File System Access API（Chrome/Edge 现代浏览器）：直接选择本地路径保存，避免内存一次性持有大 Blob */
async function handleFSA(blob: Blob, filename: string) {
  try {
    const handle = await (window as any).showSaveFilePicker({
      suggestedName: filename,
      types: [
        {
          description: '文件',
          accept: { 'application/octet-stream': ['.bin'] }
        }
      ]
    })
    const writable = await handle.createWritable()
    // FSA 下可以直接流式写入；若 blob 很大可改用 readableStream 分段写入
    await writable.write(blob)
    await writable.close()
    return
  } catch (err: any) {
    // 用户取消选择或浏览器不支持时静默处理
    if (err?.name === 'AbortError') return
    legacySave(blob, filename)
  }
}

/** 文件夹服务端打包下载（zip 流式）：folderId 为空则无意义，必须有值 */
export async function downloadFolder(folderId: number, folderName: string) {
  const store = useUserStore()
  const res = await fetch(`/api/files/download/batch?ids=${folderId}`, {
    headers: { Authorization: `Bearer ${store.token}` }
  })
  if (!res.ok) {
    let msg = '下载失败'
    try {
      const d = await res.json()
      if (d?.message) msg = d.message
    } catch {
      /* 非 JSON 错误体 */
    }
    message.error(msg)
    throw new Error(msg)
  }
  const blob = await res.blob()
  const name =
    res.headers.get('content-disposition')?.split("filename*=UTF-8''")[1]?.trim() ||
    `${folderName}.zip`
  saveBlob(blob, decodeURIComponent(name))
}

export async function downloadShared(token: string, fileId: number | null, filename: string, password?: string) {
  // 密码分享下载需携带访问密码，否则后端返回 401（B7 修复）
  const headers: Record<string, string> = {}
  if (password) headers['X-Share-Password'] = password
  const params: Record<string, number> = {}
  if (fileId) params.file_id = fileId
  const res = await request.get<Blob, AxiosResponse<Blob>>(`/share/${token}/download`, {
    params,
    headers,
    responseType: 'blob'
  })
  saveBlob(res.data, filename)
}

export async function downloadSharedFile(fileId: number, filename: string) {
  const blob = await fetchBlob('/files/shared/download', { file_id: fileId })
  saveBlob(blob, filename)
}
