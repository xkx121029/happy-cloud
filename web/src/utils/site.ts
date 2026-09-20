import { httpGet } from '@/api/request'

// 主页面公开网址（管理员在系统设置中配置）。用于分享链接等需要对外展示绝对地址的场景，
// 覆盖 window.location.origin（cloudflared 等隧道场景下各入口 origin 可能不一致）。
let cached: string | null = null
let inflight: Promise<string> | null = null

export async function getPublicBase(): Promise<string> {
  if (cached !== null) return cached
  if (!inflight) {
    inflight = (async () => {
      try {
        const res = await httpGet<{ public_url: string }>('/site-info')
        cached = (res.public_url || '').replace(/\/+$/, '')
      } catch {
        cached = ''
      }
      return cached
    })()
  }
  return inflight
}
