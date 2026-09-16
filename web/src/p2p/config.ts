import { httpGet } from '@/api/request'

export interface P2PConfig {
  p2p: boolean
  ws: string
  room: string
  iceServers: { urls: string[] }[]
}

/** 拉取后端 P2P 配置（信令地址 + ICE），失败（旧后端/未登录）返回 null */
export async function fetchP2PConfig(): Promise<P2PConfig | null> {
  try {
    return await httpGet<P2PConfig>('/p2p/config')
  } catch {
    return null
  }
}