// 离职转移接口（/api/v1/dxsf/teacher/resign/*，契约见 PLAN-web.md §4）
// 姓名/部门为冗余快照（离职后老师可能被改/删）；group_count 后端计算

import { post } from './request'
import type { PageReq, PageResp } from './types'
import { cleanQuery } from '@/utils/format'

export interface ResignRow {
  id: number
  original_teacher_id: number
  original_teacher_name: string
  original_teacher_dept_id: number
  original_teacher_dept: string
  replace_teacher_id: number
  replace_teacher_name: string
  replace_teacher_dept: string
  salesman_name: string // 原老师全部绑定业务员，逗号分隔
  salesman_dept: string
  group_count: number
  operator: string
  transfer_time: string
  transfer_content: string
  created_at: string
  updated_at: string
}

export interface ResignListQuery {
  dept_id?: number
  original_teacher?: string
  replace_teacher?: string
  salesman?: string
  transfer_begin_time?: string
  transfer_end_time?: string
}

export const listResigns = (query: ResignListQuery & PageReq) => post<PageResp<ResignRow>>('/dxsf/teacher/resign/list', cleanQuery(query))

// 新增转移：快照后端回查（姓名/部门/业务员/group_count 均不收前端值）
export const addResign = (data: { original_teacher_id: number; replace_teacher_id: number; transfer_content: string }) =>
  post<null>('/dxsf/teacher/resign/add', data)
