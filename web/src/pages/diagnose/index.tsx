// 诊股记录列表：筛选（数值 JSON number、双时间区间）+ 操作列按状态机渲染 + 详情
import { useCallback, useEffect, useState } from 'react'
import { Button, Card, DatePicker, Descriptions, Form, Input, InputNumber, Modal, Select, Table } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Dayjs } from 'dayjs'

import { listDiagnoses, type DiagnoseRow } from '@/api/diagnose'
import { DIAGNOSE_STATUS } from '@/constants/diagnose'
import { toOptions } from '@/constants/dicts'
import StatusTag from '@/components/StatusTag'
import RichTextView from '@/components/RichTextView'
import { usePagedList } from '@/hooks/usePagedList'
import { text } from '@/utils/format'
import ReportModal from './ReportModal'
import AuditModal from './AuditModal'
import DetailDrawer from './DetailDrawer'

interface QueryForm {
  id?: number
  user_nick_name?: string
  user_name?: string
  stock_code?: string
  stock_name?: string
  buy_price?: number
  buy_num?: number
  teacher_name?: string
  status?: number
  submit_range?: [Dayjs, Dayjs] | undefined
  report_range?: [Dayjs, Dayjs] | undefined
  submit_begin_time?: string
  submit_end_time?: string
  report_begin_time?: string
  report_end_time?: string
}

// 操作列按状态机渲染：1 编写报告 / 2 专业审核 / 3、5 重新提审 / 4 合规审核 / 6 查看报告；恒有详情
const actionButtons = (row: DiagnoseRow, openReport: (r: DiagnoseRow) => void, openAudit: (r: DiagnoseRow) => void, openView: (r: DiagnoseRow) => void): { key: string; label: string; onClick: () => void }[] => {
  const actions: { key: string; label: string; onClick: () => void }[] = []
  if (row.status === 1) actions.push({ key: 'write', label: '编写报告', onClick: () => openReport(row) })
  if (row.status === 2) actions.push({ key: 'pro', label: '专业审核', onClick: () => openAudit(row) })
  if (row.status === 3 || row.status === 5) actions.push({ key: 'resubmit', label: '重新提审', onClick: () => openReport(row) })
  if (row.status === 4) actions.push({ key: 'comp', label: '合规审核', onClick: () => openAudit(row) })
  if (row.status === 6) actions.push({ key: 'view', label: '查看报告', onClick: () => openView(row) })
  return actions
}

// 查看报告（终态 6）：只读富文本
const ViewReportModal = ({ row, onClose }: { row: DiagnoseRow | null; onClose: () => void }) => (
  <Modal title={`诊股报告（ID: ${row?.id ?? '-'}）`} open={row !== null} onCancel={onClose} footer={null} width={760} destroyOnHidden>
    {row && (
      <Descriptions column={2} bordered size="small" style={{ marginTop: 16 }}>
        <Descriptions.Item label="用户昵称">{text(row.user_nick_name)}</Descriptions.Item>
        <Descriptions.Item label="用户姓名">{text(row.user_name)}</Descriptions.Item>
        <Descriptions.Item label="股票">{row.stock_code} {row.stock_name}</Descriptions.Item>
        <Descriptions.Item label="买入价/持股数">{row.buy_price} / {row.buy_num}</Descriptions.Item>
        <Descriptions.Item label="报告提交时间" span={2}>{text(row.report_submit_time)}</Descriptions.Item>
        <Descriptions.Item label="诊股报告" span={2}><RichTextView html={row.report_content} /></Descriptions.Item>
      </Descriptions>
    )}
  </Modal>
)

const columns = (openReport: (r: DiagnoseRow) => void, openAudit: (r: DiagnoseRow) => void, openView: (r: DiagnoseRow) => void, openDetail: (r: DiagnoseRow) => void): ColumnsType<DiagnoseRow> => [
  { title: 'ID', dataIndex: 'id', width: 64 },
  { title: '用户昵称', dataIndex: 'user_nick_name', width: 100, ellipsis: true },
  { title: '用户姓名', dataIndex: 'user_name', width: 90, ellipsis: true },
  { title: '股票代码', dataIndex: 'stock_code', width: 90 },
  { title: '股票名称', dataIndex: 'stock_name', width: 100, ellipsis: true },
  { title: '买入价', dataIndex: 'buy_price', width: 85, align: 'right' },
  { title: '持股数', dataIndex: 'buy_num', width: 85, align: 'right' },
  { title: '老师', dataIndex: 'teacher_name', width: 90, ellipsis: true },
  { title: '提交时间', dataIndex: 'submit_time', width: 165 },
  { title: '报告提交时间', dataIndex: 'report_submit_time', width: 165, render: text },
  { title: '状态', dataIndex: 'status', width: 110, render: (v: number) => <StatusTag dict={DIAGNOSE_STATUS} value={v} /> },
  {
    title: '操作',
    key: 'action',
    width: 190,
    fixed: 'right',
    render: (_, row) => (
      <>
        {actionButtons(row, openReport, openAudit, openView).map((a) => (
          <a key={a.key} style={{ marginRight: 10 }} onClick={a.onClick}>{a.label}</a>
        ))}
        <a onClick={() => openDetail(row)}>详情</a>
      </>
    ),
  },
]

const DiagnosePage = () => {
  const [form] = Form.useForm<QueryForm>()
  const [reportRow, setReportRow] = useState<DiagnoseRow | null>(null)
  const [auditRow, setAuditRow] = useState<DiagnoseRow | null>(null)
  const [viewRow, setViewRow] = useState<DiagnoseRow | null>(null)
  const [detailRow, setDetailRow] = useState<DiagnoseRow | null>(null)

  const fetcher = useCallback((q: Parameters<typeof listDiagnoses>[0]) => listDiagnoses(q), [])
  const { list, count, loading, page, pageSize, search, reset, reload, onPaginationChange } = usePagedList<DiagnoseRow, Omit<QueryForm, 'submit_range' | 'report_range'>>(fetcher)

  const getQuery = (): Omit<QueryForm, 'submit_range' | 'report_range'> => {
    const v = form.getFieldsValue()
    const query: Omit<QueryForm, 'submit_range' | 'report_range'> = {
      id: v.id ?? undefined,
      user_nick_name: v.user_nick_name?.trim() || undefined,
      user_name: v.user_name?.trim() || undefined,
      stock_code: v.stock_code?.trim() || undefined,
      stock_name: v.stock_name?.trim() || undefined,
      buy_price: v.buy_price ?? undefined,
      buy_num: v.buy_num ?? undefined,
      teacher_name: v.teacher_name?.trim() || undefined,
      status: v.status ?? undefined,
    }
    if (v.submit_range?.[0]) {
      query.submit_begin_time = v.submit_range[0].format('YYYY-MM-DD')
      query.submit_end_time = v.submit_range[1].format('YYYY-MM-DD')
    }
    if (v.report_range?.[0]) {
      query.report_begin_time = v.report_range[0].format('YYYY-MM-DD')
      query.report_end_time = v.report_range[1].format('YYYY-MM-DD')
    }
    return query
  }

  useEffect(() => { search(getQuery) }, [])

  return (
    <Card title="诊股记录" extra={<Button icon={<ReloadOutlined />} onClick={reload} title="刷新" />}>
      <Form form={form} layout="inline" style={{ marginBottom: 16, rowGap: 12 }} onFinish={() => search(getQuery)}>
        <Form.Item name="id"><InputNumber placeholder="ID" style={{ width: 110 }} min={1} precision={0} /></Form.Item>
        <Form.Item name="user_nick_name"><Input placeholder="用户昵称（模糊）" allowClear style={{ width: 140 }} /></Form.Item>
        <Form.Item name="user_name"><Input placeholder="用户姓名（模糊）" allowClear style={{ width: 130 }} /></Form.Item>
        <Form.Item name="stock_code"><Input placeholder="股票代码（模糊）" allowClear style={{ width: 140 }} /></Form.Item>
        <Form.Item name="stock_name"><Input placeholder="股票名称（模糊）" allowClear style={{ width: 140 }} /></Form.Item>
        <Form.Item name="buy_price"><InputNumber placeholder="买入价（精确）" style={{ width: 130 }} min={0} /></Form.Item>
        <Form.Item name="buy_num"><InputNumber placeholder="持股数（精确）" style={{ width: 130 }} min={0} precision={0} /></Form.Item>
        <Form.Item name="teacher_name"><Input placeholder="老师（模糊）" allowClear style={{ width: 130 }} /></Form.Item>
        <Form.Item name="status">
          <Select placeholder="状态" allowClear style={{ width: 130 }} options={toOptions(DIAGNOSE_STATUS)} />
        </Form.Item>
        <Form.Item name="submit_range">
          <DatePicker.RangePicker placeholder={['提交开始', '提交结束']} style={{ width: 230 }} />
        </Form.Item>
        <Form.Item name="report_range">
          <DatePicker.RangePicker placeholder={['报告开始', '报告结束']} style={{ width: 230 }} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
        </Form.Item>
        <Form.Item>
          <Button onClick={() => { form.resetFields(); reset({}) }}>重置</Button>
        </Form.Item>
      </Form>

      <Table<DiagnoseRow>
        rowKey="id"
        size="middle"
        bordered
        loading={loading}
        dataSource={list}
        columns={columns(setReportRow, setAuditRow, setViewRow, setDetailRow)}
        scroll={{ x: 1500 }}
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

      <ReportModal row={reportRow} onClose={() => setReportRow(null)} onDone={() => { setReportRow(null); reload() }} />
      <AuditModal row={auditRow} onClose={() => setAuditRow(null)} onDone={() => { setAuditRow(null); reload() }} />
      <DetailDrawer row={detailRow} onClose={() => setDetailRow(null)} />
      <ViewReportModal row={viewRow} onClose={() => setViewRow(null)} />
    </Card>
  )
}

export default DiagnosePage
