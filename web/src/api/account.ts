import { httpPost } from './request'

/** 修改当前登录用户自己的密码（需旧密码） */
export function changeMyPassword(data: { old_password: string; new_password: string }) {
  return httpPost<unknown>('/auth/change-password', data)
}

/** 管理员重置指定用户密码 */
export function changeAnyPassword(data: { target_user_id: number; new_password: string }) {
  return httpPost<unknown>('/auth/change-password', data)
}
