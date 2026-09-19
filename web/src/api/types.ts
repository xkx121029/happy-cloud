export interface UserInfo {
  id: number
  username: string
  email: string
  role: 0 | 1
  quota_max: number
  quota_used: number
  status: number
  created_at: string
}

export interface FileItem {
  id: number
  parent_id: number
  name: string
  type: 0 | 1
  size: number
  hash: string
  is_shared?: 0 | 1
  is_deleted?: 0 | 1
  created_at: string
  updated_at?: string
}

export interface FileDetail {
  file: FileItem
  ref_count: number
  blob_size: number
  billed_self: boolean
  on_disk: boolean
  dedup: boolean
  storage_path: string
  migrated: boolean
}

/** 我的分享条目 */
export interface MyShareItem {
  id: number
  token: string
  file_id: number
  file_name: string
  file_size: number
  file_type: 0 | 1
  expire_at: string | null
  views: number
  password_required: boolean
  is_expired: boolean
  created_at: string
}

/** 公共共享目录条目 */
export interface SharedFileItem extends FileItem {
  is_shared: 0 | 1
  username: string
}

/** 用户摘要（私发选择接收者） */
export interface UserBrief {
  id: number
  username: string
  email: string
}

/** 文件转送记录 */
export interface TransferItem {
  id: number
  sender_id: number
  sender_name: string
  receiver_id: number
  file_id: number
  file_name: string
  file_size: number
  status: 0 | 1 | 2
  created_at: string
}

/** 通知 */
export interface NotificationItem {
  id: number
  type: string
  title: string
  content: string
  transfer_id: number
  is_read: 0 | 1
  transfer_status: number // -1 无关联转送，0 待处理 1 已接受 2 已拒绝
  created_at: string
}

export interface AdminFileItem extends FileItem {
  username?: string
}

export interface QuotaInfo {
  quota_max: number
  quota_used: number
}

export interface ShareInfo {
  token: string
  url: string
  file?: FileItem
  files?: FileItem[]
  children?: FileItem[]
  password_required: boolean
}

export interface AdminStats {
  user_count: number
  file_count: number
  storage_used: number
  today_uploads: number
  online_users: number
  dedup?: DedupStats
}

export interface DedupStats {
  blob_count: number
  physical_bytes: number
  logical_bytes: number
  saved_bytes: number
  ref_count_sum: number
  trash_only_count: number
  unbilled_count: number
  legacy_bytes: number // legacy bytes (字节数), not rows
}

export interface AdminLog {
  id: number
  user_id: number
  username: string
  action: string
  detail: string
  created_at: string
}

export interface PageData<T> {
  items?: T[]
  list?: T[]
  total: number
  page?: number
  page_size?: number
}

/** 兼容后端返回纯数组或 {list,total} 两种分页结构 */
export function toList<T>(data: T[] | PageData<T> | null | undefined): T[] {
  if (!data) return []
  if (Array.isArray(data)) return data
  // 后端统一返回 items，兼容历史 list 字段
  return data.items ?? data.list ?? []
}
