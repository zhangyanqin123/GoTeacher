// 离职转移：列表（姓名/业务员模糊、部门 ID 精确、时间范围）+ 新增转移弹窗（老师 options 双下拉）
import { useCallback, useEffect, useState } from 'react'
import { Button, Card, DatePicker, Form, Input, InputNumber, Table } from 'antd'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'

import { listResigns, type ResignRow } from '@/api/resign'
import { usePagedList } from '@/hooks/usePagedList'
import { text } from '@/utils/format'
import AddResignModal from './AddResignModal'

interface QueryForm {
  dept_id?: number
  original_teacher?: string
  replace_teacher?: string
  salesman?: string
  transfer_range?: [Dayjs, Dayjs] | undefined
  transfer_begin_time?: string
  transfer_end_time?: string
}

const columns: ColumnsType<ResignRow> = [
  { title: 'ID', dataIndex: 'id', width: 64 },
  { title: '原老师', dataIndex: 'original_teacher_name', width: 100, ellipsis: true },
  { title: '原老师部门', dataIndex: 'original_teacher_dept', width: 120, ellipsis: true, render: text },
  { title: '接收老师', dataIndex: 'replace_teacher_name', width: 100, ellipsis: true },
  { title: '接收老师部门', dataIndex: 'replace_teacher_dept', width: 120, ellipsis: true, render: text },
  { title: '转移业务员', dataIndex: 'salesman_name', ellipsis: true, render: text },
  { title: '业务员部门', dataIndex: 'salesman_dept', ellipsis: true, render: text },
  { title: '群数', dataIndex: 'group_count', width: 70, align: 'center' },
  { title: '转移内容', dataIndex: 'transfer_content', width: 110, ellipsis: true, render: text },
  { title: '操作人', dataIndex: 'operator', width: 90, ellipsis: true },
  { title: '转移时间', dataIndex: 'transfer_time', width: 165 },
]

const ResignPage = () => {
  const [form] = Form.useForm<QueryForm>()
  const [addOpen, setAddOpen] = useState(false)

  const fetcher = useCallback((q: Parameters<typeof listResigns>[0]) => listResigns(q), [])
  const { list, count, loading, page, pageSize, search, reset, reload, onPaginationChange } = usePagedList<ResignRow, Omit<QueryForm, 'transfer_range'>>(fetcher)

  const getQuery = (): Omit<QueryForm, 'transfer_range'> => {
    const v = form.getFieldsValue()
    const query: Omit<QueryForm, 'transfer_range'> = {
      dept_id: v.dept_id ?? undefined,
      original_teacher: v.original_teacher?.trim() || undefined,
      replace_teacher: v.replace_teacher?.trim() || undefined,
      salesman: v.salesman?.trim() || undefined,
    }
    if (v.transfer_range?.[0]) {
      query.transfer_begin_time = v.transfer_range[0].format('YYYY-MM-DD')
      query.transfer_end_time = v.transfer_range[1].format('YYYY-MM-DD')
    }
    return query
  }

  useEffect(() => { search(getQuery) }, [])

  return (
    <Card
      title="离职转移"
      extra={
        <>
          <Button type="primary" icon={<PlusOutlined />} style={{ marginRight: 8 }} onClick={() => setAddOpen(true)}>
            新增转移
          </Button>
          <Button icon={<ReloadOutlined />} onClick={reload} title="刷新" />
        </>
      }
    >
      <Form form={form} layout="inline" style={{ marginBottom: 16, rowGap: 12 }} onFinish={() => search(getQuery)}>
        <Form.Item name="original_teacher"><Input placeholder="原老师姓名（模糊）" allowClear style={{ width: 160 }} /></Form.Item>
        <Form.Item name="replace_teacher"><Input placeholder="接收老师姓名（模糊）" allowClear style={{ width: 160 }} /></Form.Item>
        <Form.Item name="salesman"><Input placeholder="业务员（模糊）" allowClear style={{ width: 140 }} /></Form.Item>
        <Form.Item name="dept_id"><InputNumber placeholder="原老师部门 ID" style={{ width: 140 }} min={0} precision={0} /></Form.Item>
        <Form.Item name="transfer_range">
          <DatePicker.RangePicker placeholder={['转移开始', '转移结束']} style={{ width: 240 }} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>

      <Table<ResignRow>
        rowKey="id"
        size="middle"
        bordered
        loading={loading}
        dataSource={list}
        columns={columns}
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

      <AddResignModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onDone={() => { setAddOpen(false); reload() }}
      />
    </Card>
  )
}

export default ResignPage
