// 通用格式化工具

// 发请求前剔除 ''/null/undefined 键——诊股/订单接口数值字段收到空串会 400 的关键防线
export const cleanQuery = <T extends object>(query: T): T =>
  Object.fromEntries(Object.entries(query).filter(([, v]) => v !== '' && v !== null && v !== undefined)) as T

// 可空字段后端 NULL→''，统一渲染 '-'
export const text = (v: string | number | null | undefined) => (v === '' || v === null || v === undefined ? '-' : v)

// 富文本判空：strip 标签后去空白（驳回原因必填校验用）
export const htmlIsEmpty = (html: string) => html.replace(/<[^>]*>/g, '').replace(/&nbsp;/g, ' ').trim() === ''
