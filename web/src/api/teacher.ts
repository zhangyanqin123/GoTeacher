// 老师管理接口（/api/v1/dxsf/teacher/*，7 个，契约见 PLAN-web.md §4）
// 数值筛选字段后端 FlexInt64 宽容（字符串数字也收）；status 筛选 -1=全部不过滤

import { get, post } from './request'
import type { PageReq, PageResp } from './types'
import { cleanQuery } from '@/utils/format'

export interface TeacherRow {
  id: number
  account: string
  name: string
  nickname: string
  title: string
  qualification: string // 中文展示串（已认证/未认证）
  bind_sales_count: number
  dept_id: number
  dept_name: string
  phone: string
  work_no: string
  status: string // 输出契约："1"/"0" 字符串
  rating: number
  avatar: string
  signature: string
  created_at: string
  updated_at: string
  update_by: string
}

export interface TeacherOption {
  id: number
  name: string
  dept_name: string
}

// detail/edit 字段名映射契约：列 rating/signature ↔ 接口 level/sign
export interface TeacherDetail {
  id: number
  nickname: string
  title: string
  level: number // 0 无 / 3 初级 / 5 高级
  avatar: string
  sign: string
}

export interface TeacherSalesRow {
  id: number
  username: string
  nickname: string
  dept_name: string
  bind_time: string
}

// 分页回显特例：本接口 data 是驼峰 pageIndex/pageSize（老师域唯一，勿复用 PageResp）
export interface SalesPageResp {
  list: TeacherSalesRow[]
  count: number
  pageIndex: number
  pageSize: number
}

export interface TeacherListQuery {
  dept_id?: number
  id?: number
  account?: string
  nickname?: string
  name?: string
  title?: string
  qualification?: string
  bind_sales_count?: number
  status?: number // -1 全部 / 1 启用 / 0 停用
  update_by?: string
  update_begin_time?: string
  update_end_time?: string
}

export interface TeacherUpdateReq {
  id: number
  title: string
  level: number
  avatar: string
  sign: string
}

export const listTeachers = (query: TeacherListQuery & PageReq) => post<PageResp<TeacherRow>>('/dxsf/teacher/list', cleanQuery(query))

export const listTeacherOptions = () => get<TeacherOption[]>('/dxsf/teacher/options')

export const getTeacherDetail = (id: number) => get<TeacherDetail>('/dxsf/teacher/detail', { id })

export const updateTeacher = (data: TeacherUpdateReq) => post<null>('/dxsf/teacher/edit', data)

export const listTeacherSales = (query: { id: number } & PageReq) =>
  get<SalesPageResp>('/dxsf/teacher/bind/salesman/list', query)

// 全量已绑定业务员 userId（绑定弹窗过滤参考）
export const listBoundSalesUserIds = () => get<number[]>('/dxsf/teacher/bind/salesman/users')

// 绑定业务员（追加语义：仅新增绑定，重复绑定幂等）
export const bindTeacherSales = (data: { id: number; user_ids: number[] }) =>
  post<null>('/dxsf/teacher/bind/salesman', data)
