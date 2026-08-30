// 诊股域字典：状态机 1-6 + 审核换算表（换算逻辑只放这里，页面不散落 if）
import type { DictItem } from './dicts'

// 状态枚举：1 待诊股 / 2 待专业审核 / 3 专业驳回 / 4 待合规审核 / 5 合规驳回 / 6 合规通过（终态）
export const DIAGNOSE_STATUS: DictItem<number>[] = [
  { value: 1, label: '待诊股', color: 'default' },
  { value: 2, label: '待专业审核', color: 'processing' },
  { value: 3, label: '专业驳回', color: 'error' },
  { value: 4, label: '待合规审核', color: 'processing' },
  { value: 5, label: '合规驳回', color: 'error' },
  { value: 6, label: '合规通过', color: 'success' },
]

// 可提交报告的状态（首次编写 / 驳回后重新提审），提交后统一回落 2
export const CAN_SUBMIT = [1, 3, 5]

// 审核换算表（2026-08-21 后端契约：audit 直传目标 status，白名单 3/4/5/6）
export const AUDIT_TARGET = {
  professional: { title: '专业审核', pass: 4, reject: 3 }, // 2 待专业审核 → 通过 4 / 驳回 3
  compliance: { title: '合规审核', pass: 6, reject: 5 }, // 4 待合规审核 → 通过 6 / 驳回 5
} as const

export type AuditStage = keyof typeof AUDIT_TARGET

// 状态 → 审核环节（2 → professional；4 → compliance）
export const auditStageOf = (status: number): AuditStage | null =>
  status === 2 ? 'professional' : status === 4 ? 'compliance' : null
