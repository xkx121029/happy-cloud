// P2P 管理器：单例隧道，供下载/上传/连接状态指示复用。
import { reactive, readonly } from 'vue'
import { useUserStore } from '@/stores/user'
import { fetchP2PConfig } from './config'
import { TunnelTransport } from './transport'

interface P2PState {
  enabled: boolean
  connected: boolean
  busy: boolean
  mode: 'http' | 'p2p'
  error: string
}

const state = reactive<P2PState>({
  enabled: false,
  connected: false,
  busy: false,
  mode: 'http',
  error: ''
})

let transport: TunnelTransport | null = null
let inflight: Promise<boolean> | null = null

export function useP2P() {
  // 点击指示器与设置监听会同时触发建链，这里复用同一次尝试，
  // 避免并发出两条 WebRTC 会话（一条成功、另一条失败会把状态覆盖回 HTTP）
  function connect(): Promise<boolean> {
    if (inflight) return inflight
    inflight = doConnect().finally(() => {
      inflight = null
    })
    return inflight
  }

  async function doConnect(): Promise<boolean> {
    if (transport?.connected) {
      state.connected = true
      state.enabled = true
      state.mode = 'p2p'
      state.error = ''
      return true
    }
    const store = useUserStore()
    if (!store.token) {
      state.error = '未登录'
      return false
    }
    const cfg = await fetchP2PConfig()
    if (!cfg?.p2p || !cfg.ws) {
      state.error = '后端未启用 P2P'
      return false
    }
    state.busy = true
    state.error = ''
    try {
      const t = new TunnelTransport()
      // 隧道被后端回收或 ICE 断开时，状态必须跟着回到 HTTP，
      // 否则界面会一直显示「P2P 直连」而请求全部超时（僵尸隧道）。
      t.onClosed = () => {
        if (transport !== t) return
        transport = null
        state.connected = false
        state.mode = 'http'
        state.error = 'P2P 连接已断开'
      }
      await t.connect(cfg.ws, store.token, cfg.iceServers)
      transport = t
      state.enabled = true
      state.connected = true
      state.mode = 'p2p'
      return true
    } catch (e) {
      state.error = (e as Error).message || 'P2P 连接失败'
      transport = null
      state.enabled = false
      state.connected = false
      state.mode = 'http'
      return false
    } finally {
      state.busy = false
    }
  }

  function disconnect() {
    transport?.close()
    transport = null
    state.enabled = false
    state.connected = false
    state.mode = 'http'
    state.error = ''
  }

  /** 获取可用隧道；未连接且请求时按需连接 */
  async function get(): Promise<TunnelTransport | null> {
    if (transport?.connected) return transport
    const ok = await connect()
    return ok ? transport : null
  }

  return { state: readonly(state), connect, disconnect, get }
}