import { defineStore } from 'pinia'
import { fetchMe, login as apiLogin, register as apiRegister } from '@/api/auth'
import type { UserInfo } from '@/api/types'

const TOKEN_KEY = 'happy_cloud_token'
const USER_KEY = 'happy_cloud_user'

interface UserState {
  token: string
  user: UserInfo | null
}

function readUser(): UserInfo | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? (JSON.parse(raw) as UserInfo) : null
  } catch {
    return null
  }
}

export const useUserStore = defineStore('user', {
  state: (): UserState => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: readUser()
  }),
  getters: {
    isLogin: (s) => !!s.token,
    isAdmin: (s) => s.user?.role === 1
  },
  actions: {
    setAuth(token: string, user: UserInfo) {
      this.token = token
      this.user = user
      localStorage.setItem(TOKEN_KEY, token)
      localStorage.setItem(USER_KEY, JSON.stringify(user))
    },
    async login(username: string, password: string) {
      const res = await apiLogin({ username, password })
      this.setAuth(res.token, res.user)
    },
    async register(username: string, password: string, email: string) {
      const res = await apiRegister({ username, password, email })
      this.setAuth(res.token, res.user)
    },
    async loadMe() {
      const user = await fetchMe()
      this.user = user
      localStorage.setItem(USER_KEY, JSON.stringify(user))
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    }
  }
})
