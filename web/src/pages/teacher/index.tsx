// 老师列表：筛选（模糊/精确/数值/时间范围）+ 表格 + 编辑/详情/绑定业务员弹窗
import { useCallback, useEffect, useState } from 'react'
import { AutoComplete, Button, Card, DatePicker, Form, Input, InputNumber, Select, Table, Tag } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'

import { listTeachers, type TeacherRow } from '@/api/teacher'
import { QUALIFICATION_OPTIONS, TEACHER_STATUS, TEACHER_STATUS_FILTER } from '@/constants/teacher'
import StatusTag from '@/components/StatusTag'
import { usePagedList } from '@/hooks/usePagedList'
import { text } from '@/utils/format'
import TeacherEditModal from './TeacherEditModal'
import TeacherDetailDrawer from './TeacherDetailDrawer'
import BindSalesmanModal from './BindSalesmanModal'

interface QueryForm {
  id?: number
  dept_id?: number
  account?: string
  nickname?: string
  name?: string
  title?: string
  qualification?: string
  bind_sales_count?: number
  status?: number
  update_by?: string
  update_range?: [Dayjs, Dayjs] | undefined // 拆 update_begin_time/update_end_time（YYYY-MM-DD）
  update_begin_time?: string
  update_end_time?: string
}

const columns = (onBind: (row: TeacherRow) => void, onEdit: (row: TeacherRow) => void, onDetail: (row: TeacherRow) => void): ColumnsType<TeacherRow> => [
  { title: 'ID', dataIndex: 'id', width: 64 },
  { title: '老师账号', dataIndex: 'account', width: 120, ellipsis: true },
  { title: '老师姓名', dataIndex: 'name', width: 100, ellipsis: true },
  { title: '老师昵称', dataIndex: 'nickname', width: 110, ellipsis: true },
  { title: '老师头衔', dataIndex: 'title', width: 120, ellipsis: true },
  {
    title: '执业资质',
    dataIndex: 'qualification',
    width: 90,
    render: (v: string) => (v ? <Tag color={v === '已认证' ? 'success' : 'default'}>{v}</Tag> : '-'),
  },
  { title: '绑定业务员数', dataIndex: 'bind_sales_count', width: 110, align: 'center' },
  { title: '部门', dataIndex: 'dept_name', width: 110, ellipsis: true, render: text },
  { title: '状态', dataIndex: 'status', width: 80, render: (v: string) => <StatusTag dict={TEACHER_STATUS} value={v} /> },
  { title: '更新时间', dataIndex: 'updated_at', width: 165 },
  { title: '更新人', dataIndex: 'update_by', width: 90, ellipsis: true, render: text },
  {
    title: '操作',
    key: 'action',
    width: 200,
    fixed: 'right',
    render: (_, row) => (
      <>
        <a style={{ marginRight: 12 }} onClick={() => onBind(row)}>绑定业务员</a>
        <a style={{ marginRight: 12 }} onClick={() => onEdit(row)}>编辑</a>
        <a onClick={() => onDetail(row)}>详情</a>
      </>
    ),
  },
]

const TeacherPage = () => {
  const [form] = Form.useForm<QueryForm>()
  const [editing, setEditing] = useState<TeacherRow | null>(null)
  const [editOpen, setEditOpen] = useState(false)
  const [detailRow, setDetailRow] = useState<TeacherRow | null>(null)
  const [bindRow, setBindRow] = useState<TeacherRow | null>(null)

  const fetcher = useCallback((q: Parameters<typeof listTeachers>[0]) => listTeachers(q), [])
  const { list, count, loading, page, pageSize, search, reset, reload, onPaginationChange } = usePagedList<TeacherRow, Omit<QueryForm, 'update_range'>>(fetcher)

  // 筛选表单 → 查询体（时间范围拆两键；status -1=全部不过滤）
  const getQuery = (): Omit<QueryForm, 'update_range'> => {
    const v = form.getFieldsValue()
    const query: Omit<QueryForm, 'update_range'> = {
      id: v.id ?? undefined,
      dept_id: v.dept_id ?? undefined,
      account: v.account?.trim() || undefined,
      nickname: v.nickname?.trim() || undefined,
      name: v.name?.trim() || undefined,
      title: v.title?.trim() || undefined,
      qualification: v.qualification?.trim() || undefined,
      bind_sales_count: v.bind_sales_count ?? undefined,
      update_by: v.update_by?.trim() || undefined,
    }
    if (v.status !== undefined && v.status !== -1) query.status = v.status
    if (v.update_range?.[0]) {
      query.update_begin_time = v.update_range[0].format('YYYY-MM-DD')
      query.update_end_time = v.update_range[1].format('YYYY-MM-DD')
    }
    return query
  }

  useEffect(() => { search(getQuery) }, [])

  return (
    <Card
      title="老师管理"
      extra={<Button icon={<ReloadOutlined />} onClick={reload} title="刷新" />}
    >
      <Form form={form} layout="inline" style={{ marginBottom: 16, rowGap: 12 }} onFinish={() => search(getQuery)} initialValues={{ status: -1 }}>
        <Form.Item name="id"><InputNumber placeholder="ID" style={{ width: 110 }} min={1} precision={0} /></Form.Item>
        <Form.Item name="account"><Input placeholder="老师账号（模糊）" allowClear style={{ width: 150 }} /></Form.Item>
        <Form.Item name="nickname"><Input placeholder="老师昵称（模糊）" allowClear style={{ width: 150 }} /></Form.Item>
        <Form.Item name="title"><Input placeholder="老师头衔（模糊）" allowClear style={{ width: 150 }} /></Form.Item>
        <Form.Item name="qualification">
          <AutoComplete placeholder="执业资质（精确）" allowClear options={QUALIFICATION_OPTIONS.map((q) => ({ value: q }))} style={{ width: 150 }} filterOption />
        </Form.Item>
        <Form.Item name="status">
          <Select placeholder="账号状态" style={{ width: 110 }} options={TEACHER_STATUS_FILTER} />
        </Form.Item>
        <Form.Item name="bind_sales_count"><InputNumber placeholder="绑定业务员数" style={{ width: 130 }} min={0} precision={0} /></Form.Item>
        <Form.Item name="dept_id"><InputNumber placeholder="部门 ID" style={{ width: 110 }} min={0} precision={0} /></Form.Item>
        <Form.Item name="update_by"><Input placeholder="更新人（模糊）" allowClear style={{ width: 140 }} /></Form.Item>
        <Form.Item name="update_range">
          <DatePicker.RangePicker placeholder={['更新开始', '更新结束']} style={{ width: 240 }} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); form.setFieldsValue({ status: -1 }); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>

      <Table<TeacherRow>
        rowKey="id"
        size="middle"
        bordered
        loading={loading}
        dataSource={list}
        columns={columns(setBindRow, (row) => { setEditing(row); setEditOpen(true) }, setDetailRow)}
        scroll={{ x: 1400 }}
        pagination={{
          current: page,
          pageSize,
          total: count,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          pageSizeOptions: [5, 10, 20, 50, 100],
          onChange: onPaginationChange,
        }}
      />

      {editOpen && (
        <TeacherEditModal
          open={editOpen}
          teacherId={editing?.id ?? 0}
          onClose={() => setEditOpen(false)}
          onDone={() => { setEditOpen(false); reload() }}
        />
      )}
      <TeacherDetailDrawer row={detailRow} onClose={() => setDetailRow(null)} />
      <BindSalesmanModal row={bindRow} onClose={() => setBindRow(null)} />
    </Card>
  )
}

export default TeacherPage
