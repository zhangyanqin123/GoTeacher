// 字典 Tag 渲染：<StatusTag dict={USER_STATUS} value={row.status} />
import { Tag } from 'antd'

import type { DictItem } from '@/constants/dicts'

interface Props<V> {
  dict: DictItem<V>[]
  value: V
}

const StatusTag = <V,>({ dict, value }: Props<V>) => {
  const item = dict.find((d) => d.value === value)
  return item ? <Tag color={item.color}>{item.label}</Tag> : <Tag>{String(value)}</Tag>
}

export default StatusTag
