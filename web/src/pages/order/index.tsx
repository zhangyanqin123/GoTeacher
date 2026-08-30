// 订单管理：Tabs 四页（创建订单 / 订单列表 / 积分列表 / 通知列表）
// 创建页：商品下拉（含价格库存、售罄禁用）+ 数量（上限=库存）+ 金额/可得积分实时计算；
// 下单成功跳列表页观察 stock/points/notify 三步骤异步翻转（需后端 cmd/consumer + RabbitMQ）
import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Card, Form, Input, InputNumber, Select, Table, Tabs, message } from 'antd'
import { ShoppingCartOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'

import {
  createOrder, listNotifications, listOrders, listPoints, listProducts,
  type NotificationRow, type OrderRow, type PointsRow, type Product,
} from '@/api/order'
import { IS_READ, ORDER_STATUS, ORDER_STATUS_FILTER, STEP_STATUS } from '@/constants/order'
import StatusTag from '@/components/StatusTag'
import { usePagedList } from '@/hooks/usePagedList'
import { text } from '@/utils/format'

interface OrderQueryForm { order_no?: string; product_name?: string; status?: number }
interface PointsQueryForm { order_no?: string }
interface NotifyQueryForm { title?: string }

// ---------- Tab 1：创建订单 ----------
const OrderCreate = ({ onCreated }: { onCreated: () => void }) => {
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm<{ product_id?: number; quantity: number }>()

  useEffect(() => {
    setLoading(true)
    listProducts()
      .then(setProducts)
      .catch(() => undefined)
      .finally(() => setLoading(false))
  }, [])

  // 受控重算：Form.useWatch 驱动金额/积分实时展示
  const quantity = Form.useWatch('quantity', form)
  const productId = Form.useWatch('product_id', form)
  const watched = products.find((p) => p.id === productId) ?? null
  const watchedAmount = watched ? watched.price * (quantity ?? 0) : 0

  const handleSubmit = async (values: { product_id?: number; quantity: number }) => {
    if (!values.product_id) return
    setSubmitting(true)
    try {
      await createOrder({ product_id: values.product_id, quantity: values.quantity })
      message.success('下单成功，库存/积分/通知异步处理中')
      onCreated()
    } catch {
      // 失败文案已由拦截器统一 message.error（库存不足/数量非法等）
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Form form={form} layout="vertical" style={{ maxWidth: 560 }} onFinish={handleSubmit} initialValues={{ quantity: 1 }}>
        <Form.Item name="product_id" label="商品" rules={[{ required: true, message: '请选择商品' }]}>
          <Select
            placeholder="请选择商品"
            loading={loading}
            options={products.map((p) => ({
              value: p.id,
              label: `${p.product_name}（￥${p.price}，库存 ${p.stock}）`,
              disabled: p.stock < 1,
            }))}
          />
        </Form.Item>
        <Form.Item name="quantity" label="购买数量" rules={[{ required: true, message: '请输入购买数量' }]}>
          <InputNumber
            min={1}
            max={999}
            precision={0}
            style={{ width: 160 }}
            onChange={(v) => {
              // 数量不能超过所选商品库存（后端也会拦，前端先钳制）
              if (watched && v && v > watched.stock) {
                form.setFieldsValue({ quantity: watched.stock })
                message.warning(`所选商品库存仅 ${watched.stock} 件`)
              }
            }}
          />
        </Form.Item>
        <Form.Item label="订单金额">
          <span style={{ fontSize: 18, color: '#f5222d', fontWeight: 600 }}>￥{watchedAmount.toFixed(2)}</span>
          {watched && (
            <span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>
              = ￥{watched.price} × {quantity ?? 0}（下单可得 {Math.floor(watchedAmount)} 积分）
            </span>
          )}
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<ShoppingCartOutlined />} loading={submitting}>
            立即下单
          </Button>
        </Form.Item>
      </Form>
      <Alert
        type="info"
        showIcon
        message="下单流程（MQ 异步链路）"
        description="订单先落库（MySQL），随后发布 order.created 事件（RabbitMQ fanout 广播）；扣库存 / 加积分 / 发通知由三个独立消费者异步处理。下单成功后到「订单列表」观察各步骤状态翻转。"
      />
    </>
  )
}

// ---------- Tab 2：订单列表 ----------
const stepColumn = (title: string, key: 'stock_status' | 'points_status' | 'notify_status'): ColumnsType<OrderRow>[number] => ({
  title,
  dataIndex: key,
  width: 90,
  render: (v: string) => <StatusTag dict={STEP_STATUS} value={v} />,
})

const OrderListTab = ({ refreshKey }: { refreshKey: number }) => {
  const [form] = Form.useForm<OrderQueryForm>()
  const fetcher = useCallback((q: Parameters<typeof listOrders>[0]) => listOrders(q), [])
  const { list, count, loading, page, pageSize, search, reset, onPaginationChange } = usePagedList<OrderRow, OrderQueryForm>(fetcher)

  useEffect(() => { search(getQuery) }, [refreshKey])

  const getQuery = (): OrderQueryForm => {
    const v = form.getFieldsValue()
    return {
      order_no: v.order_no?.trim() || undefined,
      product_name: v.product_name?.trim() || undefined,
      status: v.status ?? undefined,
    }
  }

  const columns: ColumnsType<OrderRow> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '订单号', dataIndex: 'order_no', width: 210 },
    { title: '商品', dataIndex: 'product_name', ellipsis: true },
    { title: '数量', dataIndex: 'quantity', width: 70, align: 'right' },
    { title: '金额', dataIndex: 'amount', width: 90, align: 'right', render: (v: number) => `￥${v.toFixed(2)}` },
    { title: '状态', dataIndex: 'status', width: 90, render: (v: string) => <StatusTag dict={ORDER_STATUS} value={v} /> },
    stepColumn('扣库存', 'stock_status'),
    stepColumn('加积分', 'points_status'),
    stepColumn('发通知', 'notify_status'),
    { title: '下单时间', dataIndex: 'created_at', width: 165 },
    { title: '更新时间', dataIndex: 'updated_at', width: 165 },
  ]

  return (
    <>
      <Form form={form} layout="inline" style={{ marginBottom: 16 }} onFinish={() => search(getQuery)}>
        <Form.Item name="order_no"><Input placeholder="订单号（精确）" allowClear style={{ width: 220 }} /></Form.Item>
        <Form.Item name="product_name"><Input placeholder="商品名称（模糊）" allowClear style={{ width: 180 }} /></Form.Item>
        <Form.Item name="status">
          <Select placeholder="状态" allowClear style={{ width: 120 }} options={ORDER_STATUS_FILTER} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit">查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>
      <Table<OrderRow>
        rowKey="id" size="middle" bordered loading={loading} dataSource={list} columns={columns} scroll={{ x: 1400 }}
        pagination={{
          current: page, pageSize, total: count, showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          pageSizeOptions: [5, 10, 20, 50, 100], onChange: onPaginationChange,
        }}
      />
    </>
  )
}

// ---------- Tab 3：积分列表 ----------
const PointsTab = () => {
  const [form] = Form.useForm<PointsQueryForm>()
  const fetcher = useCallback((q: Parameters<typeof listPoints>[0]) => listPoints(q), [])
  const { list, count, loading, page, pageSize, search, reset, onPaginationChange } = usePagedList<PointsRow, PointsQueryForm>(fetcher)

  useEffect(() => { search(getQuery) }, [])

  const getQuery = (): PointsQueryForm => {
    const v = form.getFieldsValue()
    return { order_no: v.order_no?.trim() || undefined }
  }

  const columns: ColumnsType<PointsRow> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '订单号', dataIndex: 'order_no', width: 220 },
    { title: '订单 ID', dataIndex: 'order_id', width: 90 },
    { title: '用户 ID', dataIndex: 'user_id', width: 90 },
    { title: '积分', dataIndex: 'points', width: 90, align: 'right' },
    { title: '备注', dataIndex: 'remark', ellipsis: true, render: text },
    { title: '时间', dataIndex: 'created_at', width: 165 },
  ]

  return (
    <>
      <Form form={form} layout="inline" style={{ marginBottom: 16 }} onFinish={() => search(getQuery)}>
        <Form.Item name="order_no"><Input placeholder="订单号（精确）" allowClear style={{ width: 220 }} /></Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit">查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>
      <Table<PointsRow>
        rowKey="id" size="middle" bordered loading={loading} dataSource={list} columns={columns}
        pagination={{
          current: page, pageSize, total: count, showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          pageSizeOptions: [5, 10, 20, 50, 100], onChange: onPaginationChange,
        }}
      />
    </>
  )
}

// ---------- Tab 4：通知列表 ----------
const NotifyTab = () => {
  const [form] = Form.useForm<NotifyQueryForm>()
  const fetcher = useCallback((q: Parameters<typeof listNotifications>[0]) => listNotifications(q), [])
  const { list, count, loading, page, pageSize, search, reset, onPaginationChange } = usePagedList<NotificationRow, NotifyQueryForm>(fetcher)

  useEffect(() => { search(getQuery) }, [])

  const getQuery = (): NotifyQueryForm => {
    const v = form.getFieldsValue()
    return { title: v.title?.trim() || undefined }
  }

  const columns: ColumnsType<NotificationRow> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '标题', dataIndex: 'title', ellipsis: true },
    { title: '内容', dataIndex: 'content', ellipsis: true },
    { title: '订单号', dataIndex: 'order_id', width: 90 },
    { title: '已读', dataIndex: 'is_read', width: 80, render: (v: string) => <StatusTag dict={IS_READ} value={v} /> },
    { title: '时间', dataIndex: 'created_at', width: 165 },
  ]

  return (
    <>
      <Form form={form} layout="inline" style={{ marginBottom: 16 }} onFinish={() => search(getQuery)}>
        <Form.Item name="title"><Input placeholder="标题（模糊）" allowClear style={{ width: 220 }} /></Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit">查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>
      <Table<NotificationRow>
        rowKey="id" size="middle" bordered loading={loading} dataSource={list} columns={columns}
        pagination={{
          current: page, pageSize, total: count, showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
          pageSizeOptions: [5, 10, 20, 50, 100], onChange: onPaginationChange,
        }}
      />
    </>
  )
}

// ---------- 页面组装 ----------
const OrderPage = () => {
  const [activeTab, setActiveTab] = useState('create')
  const [refreshKey, setRefreshKey] = useState(0)

  const items = [
    { key: 'create', label: '创建订单', children: <OrderCreate onCreated={() => { setRefreshKey((k) => k + 1); setActiveTab('list') }} /> },
    { key: 'list', label: '订单列表', children: <OrderListTab refreshKey={refreshKey} /> },
    { key: 'points', label: '积分列表', children: <PointsTab /> },
    { key: 'notify', label: '通知列表', children: <NotifyTab /> },
  ]

  return (
    <Card title="订单管理">
      <Tabs activeKey={activeTab} onChange={setActiveTab} type="card" items={items} />
    </Card>
  )
}

export default OrderPage
