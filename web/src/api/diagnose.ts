// 诊股接口（/api/v1/dxsf/teacher/diagnose/*，契约见 PLAN-web.md §4）
// 数值字段必须 JSON number（字符串/空串一律 400）——InputNumber + cleanQuery 保证

import { get, post } from './request'
import type { PageReq, PageResp } from './types'
import { cleanQuery } from '@/utils/format'

export interface DiagnoseRow {
  id: number
  user_nick_name: string
  user_name: string
  stock_code: string
  stock_name: string
  buy_price: number
  buy_num: number
  teacher_name: string
  submit_time: string
  report_content: string // 富文本 HTML
  report_submit_time: string // NULL→""
  status: number // 1-6
  remark: string
}

export interface DiagnoseAuditLog {
  time: string
  type: string // 中文展示串（专业审核/合规审核）
  operator: string
  result: string // 中文展示串
  remark: string
}

export interface DiagnoseDetail extends DiagnoseRow {
  audit_logs: DiagnoseAuditLog[]
}

export interface DiagnoseListQuery {
  id?: number
  user_nick_name?: string
  user_name?: string
  stock_code?: string
  stock_name?: string
  buy_price?: number
  buy_num?: number
  teacher_name?: string
  status?: number // 1-6
  submit_begin_time?: string
  submit_end_time?: string
  report_begin_time?: string
  report_end_time?: string
}

export const listDiagnoses = (query: DiagnoseListQuery & PageReq) =>
  post<PageResp<DiagnoseRow>>('/dxsf/teacher/diagnose/list', cleanQuery(query))

export const getDiagnoseDetail = (id: number) => get<DiagnoseDetail>('/dxsf/teacher/diagnose/detail', { id })

// 提交报告（状态 1/3/5 可提交，提交后回落 2）；report_content 富文本 HTML
export const submitDiagnoseReport = (data: { id: number; report_content: string }) =>
  post<null>('/dxsf/teacher/diagnose/submit/report', data)

// 审核：status 为前端换算的目标状态（3/4/5/6），驳回（3/5）时 reject_reason 必填
export const auditDiagnose = (data: { id: number; status: number; reject_reason?: string }) =>
  post<null>('/dxsf/teacher/diagnose/audit', data)
