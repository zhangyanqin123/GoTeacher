// 直播接口（小鹅通透传，公开无鉴权；凭证 access_token 由小鹅通侧校验，400 形状错 / 502 上游错）
// 注意：路径在 /guyuzhoudb 前缀（独立于 /api/v1），请求级 baseURL: '' 覆盖实例默认

import { get } from './request'

export interface XeRegisterUserResp {
  user_id: string
  user_exists: number // 1 已存在（幂等注册）
}

export interface XeLoginURLResp {
  login_url: string // 有效期仅 1 分钟，即取即用勿缓存
  permission_denied_url: string
}

export interface GetLoginURLParams {
  access_token: string
  user_id: string
  login_type: number // 1 PC / 2 H5 / 3 App
  redirect_uri?: string
}

export const registerXeUser = (params: { access_token: string; phone: string }) =>
  get<XeRegisterUserResp>('/guyuzhoudb/live/register_user', params, { baseURL: '' })

export const getXeLoginURL = (params: GetLoginURLParams) =>
  get<XeLoginURLResp>('/guyuzhoudb/live/get_login_url', params, { baseURL: '' })
