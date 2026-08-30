import axios, { type AxiosError, type AxiosResponse } from 'axios'
import { message } from 'antd'

import { ApiError, type ApiResponse } from './types'
import { clearAuth, getToken } from '@/utils/token'

// 模块扩充：silent=失败不弹全局 message（登录页/静默 getinfo 用）；rawBody=返回完整 body 不剥 data（登录特例：token 在 body 根）
declare module 'axios' {
  export interface AxiosRequestConfig {
    silent?: boolean
    rawBody?: boolean
  }
}

const instance = axios.create({ baseURL: '/api/v1', timeout: 15000 })

instance.interceptors.request.use((config) => {
  const token = getToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// 401 跳转去重：单设备互踢时并发请求会同时 401，防多次 toast / 多次跳转
let redirecting = false

instance.interceptors.response.use(
  (response: AxiosResponse): AxiosResponse => {
    const body = response.data as ApiResponse<unknown>
    // rawBody：登录特例——token/expire 在 body 根而非 data 内。
    // 返回值已剥壳为业务 data，类型上仍标注 AxiosResponse（axios 类型不支持拦截器变换响应，
    // get/post 包装处统一 as Promise<T> 收口）
    if (body.code === 200) {
      response.data = response.config.rawBody ? body : (body.data ?? null)
      return response
    }
    // HTTP 200 但业务失败（login 失败即此形态），交调用方处理
    if (!response.config.silent) message.error(body.msg || '请求失败')
    return Promise.reject(new ApiError(body.code, body.msg)) as unknown as AxiosResponse
  },
  (error: AxiosError<ApiResponse<unknown>>) => {
    const { response, config } = error
    const bodyMsg = response?.data?.msg ?? ''
    if (response?.status === 401) {
      if (!redirecting) {
        redirecting = true
        clearAuth()
        message.error(bodyMsg || '登录已过期，请重新登录')
        const redirect = encodeURIComponent(window.location.pathname + window.location.search)
        window.location.replace(`/login?redirect=${redirect}`)
        setTimeout(() => (redirecting = false), 1000)
      }
      return Promise.reject(new ApiError(401, bodyMsg || '登录已过期，请重新登录'))
    }
    const msg = bodyMsg || (error.code === 'ECONNABORTED' ? '请求超时' : '网络异常，请稍后重试')
    if (!config?.silent) message.error(msg)
    return Promise.reject(new ApiError(response?.status ?? -1, msg))
  },
)

// 泛型包装：拦截器已把业务 data 塞回 response.data，这里统一解包；页面拿到的直接是业务 data
const unwrap = <T>(p: Promise<AxiosResponse<T>>) => p.then((r) => r.data)

export const get = <T>(url: string, params?: object, config?: object): Promise<T> =>
  unwrap<T>(instance.get(url, { params, ...config }))

export const post = <T>(url: string, data?: object, config?: object): Promise<T> =>
  unwrap<T>(instance.post(url, data, config))

// 登录专用：rawBody 拿 body 根（token 在根），silent 不弹全局 message（登录页自己定位错误）
export const postRawBody = <T>(url: string, data?: object): Promise<T> =>
  unwrap<T>(instance.post(url, data, { rawBody: true, silent: true }))

export default instance
