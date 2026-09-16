import { checkHash, mergeChunks, uploadChunk, uploadDirect, uploadStatus } from '@/api/files'
import type { FileItem } from '@/api/types'
import { useSettingsStore } from '@/stores/settings'
import { useP2P } from '@/p2p'

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
  // crypto.subtle 仅在安全上下文（HTTPS 或 localhost）可用；
  // 局域网 IP（如 http://192.168.x.x）访问时降级为纯 JS 实现
  if (globalThis.crypto?.subtle) {
    const buf = await file.arrayBuffer()
    const digest = await crypto.subtle.digest('SHA-256', buf)
    return Array.from(new Uint8Array(digest))
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('')
  }
  return sha256Fallback(new Uint8Array(await file.arrayBuffer()))
}

const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
])

const SHA256_H0 = [
  0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
  0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19
]

/** 纯 JS SHA256 实现（crypto.subtle 不可用时的降级方案） */
function sha256Fallback(data: Uint8Array): string {
  const bitLenHi = Math.floor(data.length / 0x20000000)
  const bitLenLo = (data.length << 3) >>> 0
  // 填充：0x80 + 补零至 56 mod 64 + 64 位大端长度
  const paddedLen = ((data.length + 8 + 64) >> 6) << 6
  const padded = new Uint8Array(paddedLen)
  padded.set(data)
  padded[data.length] = 0x80
  const dv = new DataView(padded.buffer)
  dv.setUint32(paddedLen - 8, bitLenHi)
  dv.setUint32(paddedLen - 4, bitLenLo)

  const h = new Uint32Array(SHA256_H0)
  const w = new Uint32Array(64)
  const rotr = (x: number, n: number) => (x >>> n) | (x << (32 - n))
  const sigma0 = (x: number) => rotr(x, 7) ^ rotr(x, 18) ^ (x >>> 3)
  const sigma1 = (x: number) => rotr(x, 17) ^ rotr(x, 19) ^ (x >>> 10)
  const bigS0 = (x: number) => rotr(x, 2) ^ rotr(x, 13) ^ rotr(x, 22)
  const bigS1 = (x: number) => rotr(x, 6) ^ rotr(x, 11) ^ rotr(x, 25)
  const ch = (x: number, y: number, z: number) => (x & y) ^ (~x & z)
  const maj = (x: number, y: number, z: number) => (x & y) ^ (x & z) ^ (y & z)

  for (let off = 0; off < paddedLen; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = dv.getUint32(off + i * 4)
    for (let i = 16; i < 64; i++) {
      w[i] = (sigma1(w[i - 2]) + w[i - 7] + sigma0(w[i - 15]) + w[i - 16]) >>> 0
    }
    let [a, b, c, d, e, f, g, hh] = h
    for (let i = 0; i < 64; i++) {
      const t1 = (hh + bigS1(e) + ch(e, f, g) + SHA256_K[i] + w[i]) >>> 0
      const t2 = (bigS0(a) + maj(a, b, c)) >>> 0
      hh = g
      g = f
      f = e
      e = (d + t1) >>> 0
      d = c
      c = b
      b = a
      a = (t1 + t2) >>> 0
    }
    h[0] = (h[0] + a) >>> 0
    h[1] = (h[1] + b) >>> 0
    h[2] = (h[2] + c) >>> 0
    h[3] = (h[3] + d) >>> 0
    h[4] = (h[4] + e) >>> 0
    h[5] = (h[5] + f) >>> 0
    h[6] = (h[6] + g) >>> 0
    h[7] = (h[7] + hh) >>> 0
  }
  return Array.from(h, (x) => (x >>> 0).toString(16).padStart(8, '0')).join('')
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
 * 上传单个文件（断点续传 + 并发可配 + 内容去重）：
 * 1. SHA256 秒传校验（exists=true 直接成功）
 * 2. 小于等于 100MB 直接上传
 * 3. 大于 100MB 分片：先查 UploadStatus 跳过已上传分片，按 settings.uploadConcurrency 并发，失败后合并
 * 返回服务端落库后的文件项，供调用方立即渲染为真实文件卡片。
 */
export async function uploadFile(file: File, parentId: number, hooks: UploadHooks): Promise<FileItem | null> {
  hooks.onStatus('hashing')
  hooks.onProgress(0)
  const hash = await sha256(file)

  const check = await checkHash({ name: file.name, size: file.size, hash, parent_id: parentId })
  if (check.exists) {
    hooks.onStatus('done')
    hooks.onProgress(100)
    // 秒传：后端只回 file_id，其余字段用本地文件信息补齐
    return {
      id: check.file_id ?? 0,
      name: file.name,
      type: 1,
      size: file.size,
      parent_id: parentId,
      hash,
      created_at: new Date().toISOString()
    }
  }

  if (file.size <= LARGE_FILE_THRESHOLD) {
    hooks.onStatus('uploading')
    const form = new FormData()
    form.append('file', file)
    form.append('parent_id', String(parentId))
    const item = await uploadDirect(form, (p) => hooks.onProgress(p))
    hooks.onStatus('done')
    hooks.onProgress(100)
    return item
  }

  const total = Math.ceil(file.size / CHUNK_SIZE)
  const settings = useSettingsStore()
  const concurrency = Math.max(1, Math.min(settings.uploadConcurrency, total))

  // P2P 开启时优先走 WebRTC 隧道分片上传；失败自动降级 HTTP 分片
  let p2pFinished = false
  if (settings.p2pEnabled) {
    p2pFinished = await uploadViaP2P(file, hash, total, hooks)
    if (!p2pFinished) {
      hooks.onProgress(0)
    }
  }
  if (p2pFinished) {
    hooks.onStatus('merging')
    const merged = await mergeChunks({ hash, name: file.name, size: file.size, parent_id: parentId, chunk_total: total })
    hooks.onStatus('done')
    hooks.onProgress(100)
    return merged
  }

  // 断点续传：查询服务端已接收的分片，跳过已完成的部分
  let uploaded: number[] = []
  try {
    const status = await uploadStatus({ hash, name: file.name, size: file.size, parent_id: parentId })
    if (status.exists) {
      hooks.onStatus('merging')
      const merged = await mergeChunks({ hash, name: file.name, size: file.size, parent_id: parentId, chunk_total: total })
      hooks.onStatus('done')
      hooks.onProgress(100)
      return merged
    }
    uploaded = status.uploaded ?? []
  } catch {
    /* 服务端未接入续传时静默降级 */
  }

  const pending = Array.from({ length: total }, (_, i) => i).filter((i) => !uploaded.includes(i))
  const finished = new Array<boolean>(total).fill(false)
  for (const i of uploaded) finished[i] = true
  let finishedCount = uploaded.length

  hooks.onStatus('uploading')

  await runConcurrent(pending.length, concurrency, async (pi) => {
    const i = pending[pi]
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
  const merged = await mergeChunks({ hash, name: file.name, size: file.size, parent_id: parentId, chunk_total: total })
  hooks.onStatus('done')
  hooks.onProgress(100)
  return merged
}

/** 大文件经隧道分片上传；成功返回 true，失败返回 false（由调用方降级 HTTP） */
async function uploadViaP2P(file: File, hash: string, total: number, hooks: UploadHooks): Promise<boolean> {
  const t = await useP2P().get()
  if (!t) return false
  // 进度必须在这里自己数：之前借用 HTTP 路径的计数器（恒为 0），
  // 导致 P2P 上传时进度条一直停在 1/total 直到最后才跳到 100%。
  let done = 0
  const settings = useSettingsStore()
  const concurrency = Math.max(1, Math.min(settings.uploadConcurrency, total))
  try {
    await runConcurrent(total, concurrency, async (i) => {
      const start = i * CHUNK_SIZE
      const end = Math.min(start + CHUNK_SIZE, file.size)
      const bin = new Uint8Array(await file.slice(start, end).arrayBuffer())
      const id = (Math.random() * 0xffffff) | 0
      await t.uploadChunk(id, hash, i, bin)
      done++
      hooks.onProgress(Math.min(99, Math.round((done / total) * 100)))
    })
    return true
  } catch {
    return false
  } finally {
    hooks.onStatus('uploading')
  }
}
