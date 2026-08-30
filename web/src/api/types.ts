// 统一响应与分页契约（后端 internal/response + 各 handler 实测）
// 后端约定：{code, msg, data}，code===200 成功；列表 data.list / data.count

export interface ApiResponse<T> {
  code: number
  msg: string
  data: T
}

export interface PageReq {
  page_index: number
  page_size: number
}

export interface PageResp<T> {
  list: T[]
  count: number
}

// 业务/HTTP 失败统一 reject 的错误对象（login 页按 message 含「密码」定位输入框）
// 显式字段声明：erasableSyntaxOnly 不允许构造器参数属性语法
export class ApiError extends Error {
  code: number
  constructor(code: number, msg: string) {
    super(msg)
    this.code = code
  }
}
