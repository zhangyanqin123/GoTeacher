// 用户管理：列表（username 模糊筛选 + 分页）+ 新增/编辑弹窗 + Popconfirm 删除
import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Form, Input, message, Popconfirm, Space, Table } from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'

import { deleteAdminUser, listAdminUsers, type AdminUserRow } from '@/api/adminUser'
import { USER_STATUS } from '@/constants/dicts'
import StatusTag from '@/components/StatusTag'
import { usePagedList } from '@/hooks/usePagedList'
import { text } from '@/utils/format'
import { getUsername } from '@/utils/token'
import UserEditModal from './UserEditModal'

interface QueryForm {
  username: string
}

const columns = (currentUsername: string, onEdit: (row: AdminUserRow) => void, onDelete: (row: AdminUserRow) => void): ColumnsType<AdminUserRow> => [
  { title: 'ID', dataIndex: 'id', width: 60 },
  { title: '用户名', dataIndex: 'username', width: 140 },
  { title: '昵称', dataIndex: 'nickname', width: 140 },
  { title: '角色', dataIndex: 'role', width: 100 },
  { title: '状态', dataIndex: 'status', width: 80, render: (v: number) => <StatusTag dict={USER_STATUS} value={v} /> },
  { title: '最后登录时间', dataIndex: 'last_login_at', width: 170, render: text },
  { title: '最后登录 IP', dataIndex: 'last_login_ip', width: 140, render: text },
  { title: '创建时间', dataIndex: 'created_at', width: 170, render: text },
  {
    title: '操作',
    key: 'action',
    width: 130,
    render: (_, row) => (
      <Space>
        <a onClick={() => onEdit(row)}>编辑</a>
        {/* 不能删当前登录账号（后端也拒，前端先禁） */}
        {row.username === currentUsername ? (
          <span style={{ color: '#999', cursor: 'not-allowed' }}>删除</span>
        ) : (
          <Popconfirm title={`确定删除用户「${row.username}」？删除后该账号立即下线。`} onConfirm={() => onDelete(row)}>
            <a style={{ color: '#ff4d4f' }}>删除</a>
          </Popconfirm>
        )}
      </Space>
    ),
  },
]

const UserPage = () => {
  const [form] = Form.useForm<QueryForm>()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<AdminUserRow | null>(null)
  const currentUsername = getUsername()

  const fetcher = useCallback((q: Parameters<typeof listAdminUsers>[0]) => listAdminUsers(q), [])
  const { list, count, loading, page, pageSize, search, reset, reload, onPaginationChange } = usePagedList<AdminUserRow, QueryForm>(fetcher)

  const getQuery = (): QueryForm => {
    const values = form.getFieldsValue()
    return { username: values.username?.trim() ?? '' }
  }

  // 首屏空条件查询
  useEffect(() => { search(getQuery) }, [])

  const handleDelete = async (row: AdminUserRow) => {
    try {
      await deleteAdminUser({ id: row.id })
      message.success('删除成功')
      reload()
    } catch {
      // 失败文案已由拦截器统一 message.error（如「用户名已存在」）
    }
  }

  return (
    <Card
      title="用户管理"
      extra={
        <Space>
          <Button icon={<PlusOutlined />} type="primary" onClick={() => { setEditing(null); setModalOpen(true) }}>
            新增用户
          </Button>
          <Button icon={<ReloadOutlined />} onClick={reload} title="刷新" />
        </Space>
      }
    >
      <Form form={form} layout="inline" style={{ marginBottom: 16 }} onFinish={() => search(getQuery)}>
        <Form.Item name="username">
          <Input placeholder="用户名（模糊匹配）" allowClear style={{ width: 200 }} />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
            <Button onClick={() => { form.resetFields(); reset({ username: '' }) }}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <Table<AdminUserRow>
        rowKey="id"
        size="middle"
        bordered
        loading={loading}
        dataSource={list}
        columns={columns(currentUsername, (row) => { setEditing(row); setModalOpen(true) }, handleDelete)}
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

      <UserEditModal
        open={modalOpen}
        record={editing}
        onClose={() => setModalOpen(false)}
        onDone={() => { setModalOpen(false); reload() }}
      />
    </Card>
  )
}

export default UserPage
