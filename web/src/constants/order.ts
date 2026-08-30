// 订单域字典：订单状态与异步步骤状态（三列共用）
import type { DictItem } from './dicts'

// 订单状态（输出契约："1"/"0" 风格字符串；1 处理中 2 已完成 3 已取消）
export const ORDER_STATUS: DictItem<string>[] = [
  { value: '1', label: '处理中', color: 'processing' },
  { value: '2', label: '已完成', color: 'success' },
  { value: '3', label: '已取消', color: 'error' },
]

// 列表筛选（数字，null=不过滤）
export const ORDER_STATUS_FILTER = [
  { value: 1, label: '处理中' },
  { value: 2, label: '已完成' },
  { value: 3, label: '已取消' },
]

// 异步步骤状态（stock/points/notify 三列共用；0 待处理 1 成功 2 失败）
export const STEP_STATUS: DictItem<string>[] = [
  { value: '0', label: '待处理', color: 'default' },
  { value: '1', label: '成功', color: 'success' },
  { value: '2', label: '失败', color: 'error' },
]

// 通知已读（"1"/"0"）
export const IS_READ: DictItem<string>[] = [
  { value: '1', label: '已读', color: 'success' },
  { value: '0', label: '未读', color: 'default' },
]
