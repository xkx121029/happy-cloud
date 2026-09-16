// 隧道传输：WebRTC DataChannel 到后端。负责信令协商、R 握手鉴权，
// 以及隧道内 JSON 代理 / 下载流 / 上传分片。HTTP 直连不可达时作为加速与穿透路径。
import { FrameDecoder, encodeFrame, type Envelope } from './frames'
import { SignalingClient, type SignalMsg } from './signaling'

export type ConnMode = 'http' | 'p2p'

interface Pending {
  resolve: (r: { status: number; data: unknown }) => void
  reject: (e: Error) => void
  timer: number
}

interface StreamCb {
  onMeta: (meta: { name: string; size: number }) => void
  onData: (bin: Uint8Array, size: number) => void
  onDone: () => void
  onError: (msg: string) => void
}

export class TunnelTransport {
  mode: ConnMode = 'http'
  connected = false
  /** 隧道意外断开时回调（后端回收旧隧道、ICE 断开等），供管理器重置状态 */
  onClosed?: () => void
  private pc?: RTCPeerConnection
  private dc?: RTCDataChannel
  private signaling?: SignalingClient
  private decoder = new FrameDecoder()
  private seq = 1
  private pending = new Map<number, Pending>()
  private streams = new Map<number, StreamCb>()
  private token = ''
  private handshakeDone = false
  private handshakeFinish?: () => void
  private handshakeReject?: (e: Error) => void

  private nextId(): number {
    return this.seq++ & 0xffffff
  }

  async connect(url: string, token: string, iceServers: RTCIceServer[]): Promise<void> {
    this.token = token
    await new Promise<void>((resolve, reject) => {
      const pc = new RTCPeerConnection({ iceServers })
      this.pc = pc
      let clientId = ''
      let failed = false
      let settled = false
      let overall = 0
      const finish = () => {
        settled = true
        if (overall) clearTimeout(overall)
      }
      const fail = (msg: string) => {
        if (settled) return
        finish()
        failed = true
        this.cleanup()
        reject(new Error(msg))
      }
      // 整体超时：DataChannel 迟迟不 open（ICE 打不通）时不能让调用方永久挂住，
      // 否则 P2P 管理器里的 inflight 永不释放，用户再点指示器会毫无反应。
      overall = window.setTimeout(() => fail('P2P 连接超时（ICE 未打通）'), 20000)

      const signaling = new SignalingClient(url)
      this.signaling = signaling
      signaling.onMessage = (m: SignalMsg) => {
        if (m.t === 'welcome') {
          clientId = m.id || ''
        } else if (m.t === 'answer' && m.sdp) {
          pc.setRemoteDescription({ type: 'answer', sdp: m.sdp.sdp }).catch(() => {})
        } else if (m.t === 'ice' && m.cand) {
          pc.addIceCandidate({
            candidate: m.cand.candidate,
            sdpMid: m.cand.sdpMid || undefined,
            sdpMLineIndex: m.cand.sdpMLineIndex
          }).catch(() => {})
        } else if (m.t === 'replace' || m.t === 'host-offline') {
          // host 重载会让协商失败；交由外层重连
        }
      }
      signaling.onClose = () => {
        if (!failed && !this.connected) fail('信令连接断开')
      }

      pc.onicecandidate = (e) => {
        if (e.candidate && clientId) {
          signaling.send({
            t: 'ice',
            cid: clientId,
            cand: {
              candidate: e.candidate.candidate,
              sdpMid: e.candidate.sdpMid,
              sdpMLineIndex: e.candidate.sdpMLineIndex ?? undefined
            }
          })
        }
      }
      pc.onconnectionstatechange = () => {
        if (pc.connectionState === 'connected') {
          this.mode = this.inferMode(pc)
        } else if (pc.connectionState === 'failed') {
          // ICE 失败要尽早报错，别让用户对着「连接中」干等
          console.warn('[p2p] 连接失败（ICE 未打通）')
          this.connected = false
          this.cleanup()
          fail('P2P 连接失败（ICE 未打通）')
        }
      }
      // ICE 状态是最直接的打洞证据：checking→connected 表示直连成功，
      // failed 表示候选互不可达。出问题时看这一行即可定位。
      pc.oniceconnectionstatechange = () => {
        console.info('[p2p] ICE 状态', pc.iceConnectionState)
      }

      const dc = pc.createDataChannel('tunnel')
      this.dc = dc
      this.setupChannel(dc)

      dc.onopen = () => {
        if (failed) return
        // R 握手：token
        // b 直接给原始值，由信封的 JSON 序列化负责加引号；
        // 这里再 JSON.stringify 一次会导致后端拿到带字面引号的 "eyJ..."，token 校验必然失败。
        this.trySend({ t: 'R', id: this.nextId(), b: token })
        this.armHandshake(
          () => {
            finish()
            resolve()
          },
          (e) => {
            this.cleanup()
            finish()
            reject(e)
          },
          pc
        )
      }

      signaling.onOpen = async () => {
        try {
          const offer = await pc.createOffer()
          await pc.setLocalDescription(offer)
          signaling.send({ t: 'offer', cid: clientId, sdp: { type: offer.type ?? 'offer', sdp: offer.sdp ?? '' } })
        } catch (e) {
          fail((e as Error).message || '发起协商失败')
        }
      }

      signaling.connect()
    })
  }

  private armHandshake(resolve: () => void, reject: (e: Error) => void, pc: RTCPeerConnection) {
    this.handshakeDone = false
    this.handshakeFinish = () => {
      if (this.handshakeDone) return
      this.handshakeDone = true
      this.connected = true
      this.mode = this.inferMode(pc)
      resolve()
    }
    this.handshakeReject = (e: Error) => {
      if (this.handshakeDone) return
      this.handshakeDone = true
      reject(e)
    }
    // 兜底超时
    setTimeout(() => {
      if (!this.handshakeDone && !this.connected) {
        this.handshakeDone = true
        reject(new Error('P2P 握手超时'))
      }
    }, 15000)
  }

  private setupChannel(dc: RTCDataChannel) {
    // 后端隧道帧是二进制报文；浏览器 binaryType 默认为 'blob'，
    // 不指定 arraybuffer 时收到的会是 Blob（String(blob) → "[object Blob"），
    // 解码必然失败，表现为握手一直无响应直到超时。
    dc.binaryType = 'arraybuffer'
    dc.onmessage = (e: MessageEvent) => {
      const raw: Uint8Array = e.data instanceof ArrayBuffer ? new Uint8Array(e.data) : new TextEncoder().encode(String(e.data))
      const msg = this.decoder.feed(raw)
      if (msg) this.handleFrame(msg.env, msg.bin)
    }
    dc.onclose = () => {
      const was = this.connected
      this.connected = false
      // 只在「曾经连上」之后才通知：建链失败时的关闭由 fail 路径自行处理
      if (was) this.onClosed?.()
    }
  }

  private handleFrame(env: Envelope, bin: Uint8Array) {
    if (env.t === 'R') {
      console.info('[p2p] 隧道握手应答', env.ok ? 'ok' : env.msg || 'failed')
      if (env.ok) {
        this.handshakeFinish?.()
      } else {
        this.handshakeReject?.(new Error(env.msg || 'P2P 鉴权失败'))
      }
      return
    }
    if (env.t === 'D' && env.id != null) {
      const cb = this.streams.get(env.id)
      if (!cb) return
      if (bin.length > 0) {
        cb.onData(bin, env.s || bin.length)
      } else if (env.l) {
        cb.onDone()
        this.streams.delete(env.id)
      } else if (env.b) {
        // 元信息首帧
        try {
          const meta = typeof env.b === 'string' ? JSON.parse(env.b) : env.b
          cb.onMeta(meta as { name: string; size: number })
          this.trySend({ t: 'A', id: env.id, c: 0 })
        } catch {
          /* ignore */
        }
      }
      // 数据帧后回 ACK 释放窗口
      if (bin.length > 0) this.trySend({ t: 'A', id: env.id, c: 0 })
      return
    }
    if (env.t === 'E' && env.id != null) {
      const p = this.pending.get(env.id)
      if (p) {
        clearTimeout(p.timer)
        this.pending.delete(env.id)
        p.reject(new Error(env.msg || 'P2P 错误'))
      }
      const cb = this.streams.get(env.id)
      if (cb) {
        cb.onError(env.msg || '传输错误')
        this.streams.delete(env.id)
      }
      return
    }
    if ((env.t === 'J' || env.t === 'A') && env.id != null) {
      const p = this.pending.get(env.id)
      if (p) {
        clearTimeout(p.timer)
        this.pending.delete(env.id)
        let data: unknown = null
        try {
          data = env.b && typeof env.b === 'string' ? JSON.parse(env.b) : env.b
        } catch {
          data = env.b
        }
        p.resolve({ status: env.status || 0, data })
      }
    }
  }

  private inferMode(pc: RTCPeerConnection): ConnMode {
    return 'p2p'
  }

  private trySend(env: Envelope, bin?: Uint8Array) {
    if (!this.dc || this.dc.readyState !== 'open') return
    for (const pkt of encodeFrame(env, bin)) {
      this.dc.send(pkt.buffer.slice(pkt.byteOffset, pkt.byteOffset + pkt.byteLength) as ArrayBuffer)
    }
  }

  /** 隧道内 JSON HTTP 代理 */
  request(method: string, path: string, query?: Record<string, string>, body?: unknown): Promise<{ status: number; data: unknown }> {
    const id = this.nextId()
    const env: Envelope = { t: 'j', id, m: method, p: path, q: query }
    if (body != null) {
      // 同理交给信封序列化，避免二次编码后后端把 HTTP body 读成 JSON 字符串
      env.b = body
    }
    return new Promise((resolve, reject) => {
      const timer = window.setTimeout(() => {
        if (this.pending.has(id)) {
          this.pending.delete(id)
          reject(new Error('P2P 请求超时'))
        }
      }, 30000)
      this.pending.set(id, { resolve, reject, timer })
      this.trySend(env)
    })
  }

  /** 开启文件下载流（d 帧）；off/len 非空时发区间下载 */
  downloadStream(fileId: number, id: number, cb: StreamCb, off?: number, len?: number) {
    this.streams.set(id, cb)
    const env: Envelope = { t: 'd', id, dl: fileId }
    if (off != null && off >= 0) env.off = off
    if (len != null && len >= 0) env.len = len
    this.trySend(env)
  }

  /** 上传一个分片（U 帧），等 A 确认 */
  uploadChunk(id: number, hash: string, idx: number, bin: Uint8Array): Promise<void> {
    return new Promise((resolve, reject) => {
      const timer = window.setTimeout(() => {
        if (this.pending.has(id)) {
          this.pending.delete(id)
          reject(new Error('P2P 上传分片超时'))
        }
      }, 30000)
      this.pending.set(id, {
        resolve: () => resolve(),
        reject,
        timer
      })
      this.trySend({ t: 'U', id, hash, idx, s: bin.length }, bin)
    })
  }

  close() {
    this.connected = false
    this.cleanup()
  }

  private cleanup() {
    for (const p of this.pending.values()) clearTimeout(p.timer)
    this.pending.clear()
    this.streams.clear()
    try {
      this.signaling?.close()
    } catch {
      /* ignore */
    }
    try {
      this.dc?.close()
    } catch {
      /* ignore */
    }
    try {
      this.pc?.close()
    } catch {
      /* ignore */
    }
  }
}