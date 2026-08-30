// 用户管理字典（admin_user.status 是 number 1/0——注意与 teacher.status 字符串不同，分开建）

export interface DictItem<V> {
  value: V
  label: string
  color: string
}

// 派生下拉 options
export const toOptions = <V,>(dict: DictItem<V>[]) => dict.map(({ value, label }) => ({ value, label }))

export const USER_STATUS: DictItem<number>[] = [
  { value: 1, label: '启用', color: 'success' },
  { value: 0, label: '停用', color: 'default' },
]
