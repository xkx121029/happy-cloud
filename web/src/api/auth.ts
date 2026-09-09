import { httpGet, httpPost } from './request'
import type { UserInfo } from './types'

export interface AuthResult {
  token: string
  user: UserInfo
}

export function login(data: { username: string; password: string }) {
  return httpPost<AuthResult>('/auth/login', data)
}

export function register(data: { username: string; password: string; email?: string }) {
  return httpPost<AuthResult>('/auth/register', data)
}

export function fetchMe() {
  return httpGet<UserInfo>('/auth/me')
}
