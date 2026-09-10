import { httpGet, httpPut } from './request'

export interface UserSettingsPayload {
  settings: string
}

export function fetchUserSettings(): Promise<UserSettingsPayload> {
  return httpGet<UserSettingsPayload>('/user/settings')
}

export function saveUserSettings(settings: string) {
  return httpPut<unknown>('/user/settings', { settings })
}
