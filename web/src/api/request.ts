import axios, { type AxiosRequestConfig } from 'axios'
import { message } from '@/utils/notify'
import { useUserStore } from '@/stores/user'

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

const request = axios.create({
  baseURL: '/api',
  timeout: 120000
})

request.interceptors.request.use((config) => {
  const store = useUserStore()
  if (store.token) {
    config.headers.Authorization = `Bearer ${store.token}`
  }
  return config
})

function redirectToLogin() {
  const store = useUserStore()
  store.logout()
  if (!window.location.pathname.startsWith('/login')) {
    window.location.href = '/login'
  }
}

request.interceptors.response.use(
  (response): any => {
    // blob（下载流）不参与统一 JSON 结构校验
    if (response.config.responseType === 'blob') {
      return response
    }
    const res = response.data as ApiResponse
    if (res.code !== 0) {
      if (res.code === 401) {
        redirectToLogin()
      }
      message.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message || '请求失败'))
    }
    return res
  },
  async (error): Promise<any> => {
    let msg = '网络异常，请稍后重试'
    const data = error.response?.data
    if (data instanceof Blob) {
      try {
        const parsed = JSON.parse(await data.text())
        if (parsed?.message) msg = parsed.message
      } catch {
        /* 非 JSON 错误体，忽略 */
      }
    } else if (data?.message) {
      msg = data.message
    } else if (error.response?.status === 401) {
      msg = '登录已过期，请重新登录'
      redirectToLogin()
    } else if (error.response?.status === 403) {
      msg = '没有权限执行此操作'
    } else if (error.response?.status === 404) {
      msg = '请求的资源不存在'
    } else if (error.response?.status === 500) {
      msg = '服务器内部错误'
    }
    message.error(msg)
    return Promise.reject(new Error(msg))
  }
)

export async function httpGet<T>(url: string, params?: object): Promise<T> {
  const res = await request.get<unknown, ApiResponse<T>>(url, { params })
  return res.data
}

export async function httpPost<T>(url: string, data?: object): Promise<T> {
  const res = await request.post<unknown, ApiResponse<T>>(url, data)
  return res.data
}

export async function httpPatch<T>(url: string, data?: object): Promise<T> {
  const res = await request.patch<unknown, ApiResponse<T>>(url, data)
  return res.data
}

export async function httpDelete<T>(url: string, data?: object): Promise<T> {
  const res = await request.delete<unknown, ApiResponse<T>>(url, { data })
  return res.data
}

/** multipart 上传（自动带进度回调，不手动设置 Content-Type 以保留 boundary） */
export async function httpUpload<T>(url: string, form: FormData, onProgress?: (percent: number) => void): Promise<T> {
  const config: AxiosRequestConfig = {
    onUploadProgress: (e) => {
      if (onProgress && e.total) {
        onProgress(Math.min(99, Math.round((e.loaded / e.total) * 100)))
      }
    }
  }
  const res = await request.post<unknown, ApiResponse<T>>(url, form, config)
  return res.data
}

export default request
