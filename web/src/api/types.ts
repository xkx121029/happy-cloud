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
  password_required: boolean
}

export interface AdminStats {
  user_count: number
  file_count: number
  storage_used: number
  today_uploads: number
  online_users: number
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
  list: T[]
  total: number
  page?: number
  page_size?: number
}

/** 兼容后端返回纯数组或 {list,total} 两种分页结构 */
export function toList<T>(data: T[] | PageData<T> | null | undefined): T[] {
  if (!data) return []
  if (Array.isArray(data)) return data
  return data.list ?? []
}
