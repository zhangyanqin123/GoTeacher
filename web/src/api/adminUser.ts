// 用户管理接口（/api/v1/admin/user/*，契约见 PLAN-web.md §4）
// password 永不返回（后端 json:"-"）；编辑时 password 空串=不修改；不能删当前登录账号

import { post } from './request'
import type { PageReq, PageResp } from './types'
import { cleanQuery } from '@/utils/format'

export interface AdminUserRow {
  id: number
  username: string
  nickname: string
  role: string
  avatar: string
  status: number // 1 启用 / 0 停用（number，与 teacher 字符串不同）
  last_login_at: string
  last_login_ip: string
  created_at: string
  updated_at: string
}

export interface AdminUserListQuery {
  username: string // 模糊，空串不过滤
}

export const listAdminUsers = (query: AdminUserListQuery & PageReq) =>
  post<PageResp<AdminUserRow>>('/admin/user/list', cleanQuery(query))

export const addAdminUser = (data: { username: string; password: string }) => post<null>('/admin/user/add', data)

export const editAdminUser = (data: { id: number; username: string; password: string }) =>
  post<null>('/admin/user/edit', data)

export const deleteAdminUser = (data: { id: number }) => post<null>('/admin/user/delete', data)
