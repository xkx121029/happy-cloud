// 隧道帧编解码：与后端 internal/p2p/frames.go 对齐
// 逻辑流 = [4B BE msgLen][envJSON][bin]；>16KB 拆多 packet，每 packet 带 frag 头。
export const FRAG_LIMIT = 16000

export interface Envelope {
  v?: string
  t: string
  id?: number
  m?: string
  p?: string
  q?: Record<string, string>
  h?: Record<string, string>
  b?: unknown // body（j 的请求体／R 的 token，直接给原始值，由本层序列化）
  s?: number // size
  c?: number // ack 码 / 上传分片 idx
  msg?: string
  l?: boolean // 下载最后帧
  dl?: number // 下载文件 id
  hash?: string
  idx?: number
  tot?: number
  ts?: number
  ok?: boolean
  status?: number // http 状态
  f?: number // 分片序号
  fn?: number // 分片总数
  off?: number // 区间下载起始偏移
  len?: number // 区间下载长度
}

function buildPacket(frag: Uint8Array, chunk: Uint8Array): Uint8Array {
  const buf = new Uint8Array(8 + frag.length + chunk.length)
  const dv = new DataView(buf.buffer)
  dv.setUint32(0, frag.length)
  buf.set(frag, 4)
  dv.setUint32(4 + frag.length, chunk.length)
  buf.set(chunk, 8 + frag.length)
  return buf
}

/** 编码一条逻辑消息为若干 packet */
export function encodeFrame(env: Envelope, bin?: Uint8Array): Uint8Array[] {
  const envB = new TextEncoder().encode(JSON.stringify(env))
  const stream = new Uint8Array(4 + envB.length + (bin ? bin.length : 0))
  const dv = new DataView(stream.buffer)
  dv.setUint32(0, envB.length)
  stream.set(envB, 4)
  if (bin) stream.set(bin, 4 + envB.length)

  if (stream.length <= FRAG_LIMIT) {
    return [buildPacket(new TextEncoder().encode(JSON.stringify({ t: env.t, id: env.id })), stream)]
  }
  const n = Math.ceil(stream.length / FRAG_LIMIT)
  const out: Uint8Array[] = []
  for (let i = 0; i < n; i++) {
    const start = i * FRAG_LIMIT
    const end = Math.min(start + FRAG_LIMIT, stream.length)
    const frag = new TextEncoder().encode(JSON.stringify({ t: env.t, id: env.id, f: i, fn: n }))
    out.push(buildPacket(frag, stream.subarray(start, end)))
  }
  return out
}

/** 聚合器：把 DataChannel 收到的 packet 重组成完整逻辑消息 */
export class FrameDecoder {
  private partial = new Map<string, { chunks: Uint8Array[]; count: number; need: number }>()

  feed(pkt: Uint8Array): { env: Envelope; bin: Uint8Array } | null {
    if (pkt.length < 8) return null
    const dv = new DataView(pkt.buffer, pkt.byteOffset, pkt.byteLength)
    const lj = dv.getUint32(0)
    if (pkt.length < 4 + lj + 4) return null
    let frag: Envelope
    try {
      frag = JSON.parse(new TextDecoder().decode(pkt.subarray(4, 4 + lj)))
    } catch {
      return null
    }
    const lc = dv.getUint32(4 + lj)
    if (pkt.length < 8 + lj + lc) return null
    const chunk = pkt.subarray(8 + lj, 8 + lj + lc)

    if (!frag.fn) {
      return this.parseStream(chunk)
    }
    const key = `${frag.t}|${frag.id}`
    let e = this.partial.get(key)
    if (!e) {
      e = { chunks: [], count: 0, need: frag.fn }
      this.partial.set(key, e)
    }
    e.chunks.push(chunk)
    e.count++
    if (e.count >= e.need) {
      this.partial.delete(key)
      const total = e.chunks.reduce((s, c) => s + c.length, 0)
      const stream = new Uint8Array(total)
      let off = 0
      for (const c of e.chunks) {
        stream.set(c, off)
        off += c.length
      }
      return this.parseStream(stream)
    }
    return null
  }

  private parseStream(stream: Uint8Array): { env: Envelope; bin: Uint8Array } | null {
    if (stream.length < 4) return null
    const dv = new DataView(stream.buffer, stream.byteOffset, stream.byteLength)
    const msgLen = dv.getUint32(0)
    if (stream.length < 4 + msgLen) return null
    let env: Envelope
    try {
      env = JSON.parse(new TextDecoder().decode(stream.subarray(4, 4 + msgLen)))
    } catch {
      return null
    }
    return { env, bin: stream.subarray(4 + msgLen) }
  }
}