// 鉴权接口（login/logout/getinfo，契约见 PLAN-web.md §4）
// login 特例：失败也 HTTP 200 + code 400；成功响应 token/expire/passwd_expired 在 body 根而非 data 内

import { get, post, postRawBody } from './request'
import type { ApiResponse } from './types'

export interface LoginBody extends ApiResponse<null> {
  token: string
  expire: number
  passwd_expired: boolean
}

export interface UserInfo {
  roles: string[]
  name: string
  avatar: string
  introduction: string
  permissions: string[]
}

// 登录：postRawBody 拿完整 body（token 在根）+ silent（登录页按文案含「密码」定位密码框）
export const login = (data: { username: string; password: string }) => postRawBody<LoginBody>('/login', data)

export const logout = () => post<null>('/logout')

export const getInfo = () => get<UserInfo>('/getinfo', undefined, { silent: true })
