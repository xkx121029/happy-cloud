import { httpGet, httpPost } from './request'
import type { FileItem, TransferItem, UserBrief } from './types'

/** 搜索可发送的用户 */
export function searchUsers(keyword: string) {
  return httpGet<UserBrief[]>('/transfer/users', { keyword })
}

/** 发送文件给用户 */
export function sendTransfer(data: { receiver_id: number; file_id: number }) {
  return httpPost<TransferItem>('/transfer/send', data)
}

/** 我收到的转送列表 */
export function incomingTransfers() {
  return httpGet<TransferItem[]>('/transfer/incoming')
}

/** 接受转送（转存到我的网盘） */
export function acceptTransfer(id: number) {
  return httpPost<FileItem>(`/transfer/${id}/accept`)
}

/** 拒绝转送 */
export function rejectTransfer(id: number) {
  return httpPost<TransferItem>(`/transfer/${id}/reject`)
}
