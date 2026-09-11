import { defineStore } from 'pinia'
import { fetchUserSettings, saveUserSettings } from '@/api/settings'

const LS_KEY = 'happy_cloud_settings'

export interface SettingsState {
  /** 主题：light / dark */
  theme: 'light' | 'dark'
  /** 主页默认文件视图：grid / list */
  defaultView: 'grid' | 'list'
  /** 默认排序：name / date / size */
  defaultSort: 'name' | 'date' | 'size'
  /** 每页数量（分页/加载更多） */
  pageSize: number
  /** 并行上传数 */
  uploadConcurrency: number
  /** 记住我：登录页记住用户名（M11 修复：移除明文密码字段） */
  remember: { username: string; enabled: boolean }
  /** 是否已从后端加载 */
  loaded: boolean
}

const DEFAULT_STATE: SettingsState = {
  theme: 'light',
  defaultView: 'grid',
  defaultSort: 'name',
  pageSize: 50,
  uploadConcurrency: 3,
  remember: { username: '', enabled: false },
  loaded: false
}

function readLS(): Partial<SettingsState> {
  try {
    const raw = localStorage.getItem(LS_KEY)
    return raw ? (JSON.parse(raw) as Partial<SettingsState>) : {}
  } catch {
    return {}
  }
}

function writeLS(s: Partial<SettingsState>) {
  const current = readLS()
  localStorage.setItem(LS_KEY, JSON.stringify({ ...current, ...s }))
}

/** 应用主题到 document：给根元素加 .hc-dark 类 */
export function applyTheme(theme: 'light' | 'dark') {
  if (typeof document === 'undefined') return
  if (theme === 'dark') document.documentElement.classList.add('hc-dark')
  else document.documentElement.classList.remove('hc-dark')
}

export const useSettingsStore = defineStore('settings', {
  state: (): SettingsState => {
    const ls = readLS()
    // M11 修复：迁移旧数据，清除 localStorage 中已持久化的明文密码字段
    const remember = ls.remember as ({ username?: string; password?: string; enabled?: boolean } | undefined)
    if (remember && 'password' in remember) {
      delete (remember as Record<string, unknown>).password
      writeLS({ remember: { username: remember.username ?? '', enabled: remember.enabled ?? false } })
    }
    return { ...DEFAULT_STATE, ...ls }
  },
  actions: {
    /** 从后端加载（懒加载：登录后再调一次） */
    async loadFromBackend() {
      try {
        const { settings } = await fetchUserSettings()
        if (settings) {
          const parsed = JSON.parse(settings) as Partial<SettingsState>
          // M11 修复：从后端加载时同样剔除旧版本持久化的明文密码，防止回灌到本地
          const remember = parsed.remember as ({ username?: string; password?: string; enabled?: boolean } | undefined)
          if (remember && 'password' in remember) {
            delete (remember as Record<string, unknown>).password
          }
          Object.assign(this, parsed)
          writeLS(parsed)
          this.loaded = true
        }
      } catch {
        /* 未登录/后端不可用时忽略 */
      }
    },
    /** 存当前状态到 localStorage + 后端（登录态时） */
    async persist() {
      writeLS(this.$state as Partial<SettingsState>)
      try {
        const { useUserStore } = await import('./user')
        const userStore = useUserStore()
        if (userStore.isLogin) {
          await saveUserSettings(JSON.stringify({ ...this.$state, loaded: true }))
        }
      } catch {
        /* 忽略 */
      }
    },
    /** 更新单个字段并持久化 */
    async set<K extends keyof SettingsState>(key: K, value: SettingsState[K]) {
      ;(this as any)[key] = value
      // 立即应用（主题）
      if (key === 'theme' && (value === 'light' || value === 'dark')) applyTheme(value)
      await this.persist()
    }
  }
})
