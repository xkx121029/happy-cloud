// 独立信令服务器的 WebSocket 客户端（client 角色）
export interface SignalMsg {
  t: string // welcome/offer/answer/ice/ping/pong/replace/host-offline/error
  id?: string
  cid?: string
  to?: string
  sdp?: { type: string; sdp: string }
  cand?: { candidate: string; sdpMid: string | null; sdpMLineIndex?: number }
  msg?: string
}

export class SignalingClient {
  private ws?: WebSocket
  private closeByUser = false
  onMessage?: (m: SignalMsg) => void
  onOpen?: () => void
  onClose?: () => void

  constructor(private url: string) {}

  connect() {
    this.closeByUser = false
    this.ws = new WebSocket(this.url)
    this.ws.onopen = () => {
      this.onOpen?.()
    }
    this.ws.onmessage = (e) => {
      try {
        const m = JSON.parse(e.data) as SignalMsg
        this.onMessage?.(m)
      } catch {
        /* ignore */
      }
    }
    this.ws.onerror = () => {
      /* 由 onClose/重试处理 */
    }
    this.ws.onclose = () => {
      if (!this.closeByUser) this.onClose?.()
    }
  }

  send(m: SignalMsg) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(m))
    }
  }

  close() {
    this.closeByUser = true
    try {
      this.ws?.close()
    } catch {
      /* ignore */
    }
  }
}