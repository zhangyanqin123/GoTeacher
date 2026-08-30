// 订单接口（订单 Demo：Gin → MySQL → RabbitMQ 异步链路，见 PLAN-web.md §4）
// status 及 *_status 输出为字符串 "1"/"0"；列表筛选 status 必须数字（白名单 1/2/3）

import { get, post } from './request'
import type { PageReq, PageResp } from './types'
import { cleanQuery } from '@/utils/format'

export interface Product {
  id: number
  product_name: string
  price: number
  stock: number
}

export interface OrderRow {
  id: number
  order_no: string
  user_id: number
  product_id: number
  product_name: string
  quantity: number
  amount: number
  status: string // "1"处理中 / "2"已完成 / "3"已取消
  stock_status: string // "0"待处理 / "1"成功 / "2"失败
  points_status: string
  notify_status: string
  created_at: string
  updated_at: string
}

export interface PointsRow {
  id: number
  user_id: number
  order_id: number
  order_no: string
  points: number
  remark: string
  created_at: string
}

export interface NotificationRow {
  id: number
  user_id: number
  order_id: number
  title: string
  content: string
  is_read: string // "1"/"0"
  created_at: string
}

export interface OrderCreateReq {
  product_id: number
  quantity: number // 1-999，上限=所选商品库存
}

export interface OrderListQuery {
  order_no?: string // 精确
  product_name?: string // 模糊（快照列）
  status?: number // 1/2/3，未填不过滤
}

export interface PointsListQuery {
  order_no?: string // 精确（快照列）
}

export interface NotificationListQuery {
  title?: string // 模糊
}

export const createOrder = (data: OrderCreateReq) => post<OrderRow>('/orders', data)

export const listOrders = (query: OrderListQuery & PageReq) => post<PageResp<OrderRow>>('/orders/list', cleanQuery(query))

// 全量商品（含价格/库存），data 直接为数组不分页
export const listProducts = () => get<Product[]>('/orders/products')

export const listPoints = (query: PointsListQuery & PageReq) => post<PageResp<PointsRow>>('/points/list', cleanQuery(query))

export const listNotifications = (query: NotificationListQuery & PageReq) =>
  post<PageResp<NotificationRow>>('/notifications/list', cleanQuery(query))
