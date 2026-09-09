import { checkHash, mergeChunks, uploadChunk, uploadDirect } from '@/api/files'

export const CHUNK_SIZE = 10 * 1024 * 1024
export const LARGE_FILE_THRESHOLD = 100 * 1024 * 1024

export type UploadStatus = 'hashing' | 'uploading' | 'merging' | 'done' | 'error'

export interface UploadTaskItem {
  id: number
  name: string
  size: number
  status: UploadStatus
  progress: number
}

export interface UploadHooks {
  onStatus: (status: UploadStatus) => void
  onProgress: (percent: number) => void
}

async function sha256(file: File): Promise<string> {
  const buf = await file.arrayBuffer()
  const digest = await crypto.subtle.digest('SHA-256', buf)
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

async function runConcurrent(total: number, limit: number, task: (i: number) => Promise<void>) {
  let index = 0
  const worker = async () => {
    while (index < total) {
      const i = index++
      await task(i)
    }
  }
  const count = Math.min(limit, total)
  await Promise.all(Array.from({ length: count }, () => worker()))
}

/**
 * 上传单个文件：
 * 1. SHA256 秒传校验（exists=true 直接成功）
 * 2. 小于等于 100MB 直接上传
 * 3. 大于 100MB 分片（每片 10MB）3 并发上传后合并
 */
export async function uploadFile(file: File, parentId: number, hooks: UploadHooks): Promise<void> {
  hooks.onStatus('hashing')
  hooks.onProgress(0)
  const hash = await sha256(file)

  const check = await checkHash({ name: file.name, size: file.size, hash, parent_id: parentId })
  if (check.exists) {
    hooks.onStatus('done')
    hooks.onProgress(100)
    return
  }

  if (file.size <= LARGE_FILE_THRESHOLD) {
    hooks.onStatus('uploading')
    const form = new FormData()
    form.append('file', file)
    form.append('parent_id', String(parentId))
    await uploadDirect(form, (p) => hooks.onProgress(p))
    hooks.onStatus('done')
    hooks.onProgress(100)
    return
  }

  const total = Math.ceil(file.size / CHUNK_SIZE)
  const finished = new Array<boolean>(total).fill(false)
  let finishedCount = 0

  hooks.onStatus('uploading')
  await runConcurrent(total, 3, async (i) => {
    const start = i * CHUNK_SIZE
    const end = Math.min(start + CHUNK_SIZE, file.size)
    const slice = file.slice(start, end)
    const form = new FormData()
    form.append('file', slice, file.name)
    form.append('hash', hash)
    form.append('chunk_index', String(i))
    form.append('chunk_total', String(total))
    await uploadChunk(form, (loaded) => {
      const base = finishedCount / total
      const cur = loaded / 100 / total
      hooks.onProgress(Math.min(99, Math.round((base + cur) * 100)))
    })
    finished[i] = true
    finishedCount++
    hooks.onProgress(Math.min(99, Math.round((finishedCount / total) * 100)))
  })

  hooks.onStatus('merging')
  await mergeChunks({ hash, name: file.name, size: file.size, parent_id: parentId, chunk_total: total })
  hooks.onStatus('done')
  hooks.onProgress(100)
}
