import { httpGet, httpPost } from './request'
import type { NotificationItem } from './types'

export interface NotifyPage {
  items: NotificationItem[]
  total: number
  page: number
  page_size: number
}

/** 通知列表 */
export function listNotifications(page = 1, pageSize = 20) {
  return httpGet<NotifyPage>('/notifications/list', { page, page_size: pageSize })
}

/** 标记单条通知已读 */
export function readNotification(id: number) {
  return httpPost<unknown>(`/notifications/${id}/read`)
}

/** 未读通知数 */
export function unreadCount() {
  return httpGet<{ count: number }>('/notifications/unread')
}
