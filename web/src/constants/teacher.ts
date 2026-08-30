// 老师域字典（teacher.status 是字符串 '1'/'0'——与 admin_user 的 number 不同，分开建）
import type { DictItem } from './dicts'

// 账号状态（输出契约："1"/"0" 字符串）
export const TEACHER_STATUS: DictItem<string>[] = [
  { value: '1', label: '启用', color: 'success' },
  { value: '0', label: '停用', color: 'default' },
]

// 列表筛选下拉（-1 全部=后端不过滤契约；1 启用 / 0 停用）
export const TEACHER_STATUS_FILTER = [
  { value: -1, label: '全部' },
  { value: 1, label: '启用' },
  { value: 0, label: '停用' },
]

// 评级 level（detail/edit 契约：0 无 / 3 初级 / 5 高级）
export const TEACHER_LEVEL: DictItem<number>[] = [
  { value: 0, label: '无', color: 'default' },
  { value: 3, label: '初级', color: 'processing' },
  { value: 5, label: '高级', color: 'gold' },
]

// 执业资质（库里自由中文串、精确匹配；AutoComplete 可输可选）
export const QUALIFICATION_OPTIONS = ['已认证', '未认证']
