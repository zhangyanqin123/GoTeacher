// 绑定业务员弹窗：上半已绑定业务员分页小表格（默认 5）；下半 tags 手输 user_ids 提交绑定。
// 后端无业务员候选列表接口（旧版人员树来自远程 mock），故手动输入数字 ID（追加语义幂等）
import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Modal, Select, Table, message } from 'antd'

import { bindTeacherSales, listBoundSalesUserIds, listTeacherSales, type TeacherRow, type TeacherSalesRow } from '@/api/teacher'
import { text } from '@/utils/format'

const BindSalesmanModal = ({ row, onClose }: { row: TeacherRow | null; onClose: () => void }) => {
  const [list, setList] = useState<TeacherSalesRow[]>([])
  const [count, setCount] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [userIds, setUserIds] = useState<string[]>([])
  const [boundIds, setBoundIds] = useState<number[]>([])
  const [saving, setSaving] = useState(false)

  const fetchSales = useCallback((teacherId: number, pageIndex: number) => {
    setLoading(true)
    listTeacherSales({ id: teacherId, page_index: pageIndex, page_size: 5 })
      .then((resp) => {
        setList(resp.list ?? [])
        setCount(resp.count ?? 0)
      })
      .catch(() => undefined)
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!row) return
    setPage(1)
    setUserIds([])
    fetchSales(row.id, 1)
    listBoundSalesUserIds().then(setBoundIds).catch(() => undefined)
  }, [row, fetchSales])

  const handleBind = async () => {
    if (!row || userIds.length === 0) return
    const ids = userIds.map(Number)
    if (ids.some((n) => !Number.isInteger(n) || n <= 0)) {
      message.warning('user_ids 必须为正整数')
      return
    }
    setSaving(true)
    try {
      await bindTeacherSales({ id: row.id, user_ids: ids })
      message.success('绑定成功')
      setUserIds([])
      fetchSales(row.id, page)
      listBoundSalesUserIds().then(setBoundIds).catch(() => undefined)
    } catch {
      // 失败文案已由拦截器统一 message.error
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={`绑定业务员（${row?.name ?? ''}，ID: ${row?.id ?? '-'}）`}
      open={row !== null}
      onCancel={onClose}
      footer={null}
      width={720}
      destroyOnHidden
    >
      <Table
        rowKey="id"
        size="small"
        bordered
        loading={loading}
        dataSource={list}
        pagination={{
          current: page,
          pageSize: 5,
          total: count,
          size: 'small',
          showTotal: (t) => `共 ${t} 名`,
          onChange: (p) => { setPage(p); if (row) fetchSales(row.id, p) },
        }}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 70 },
          { title: '用户名', dataIndex: 'username', ellipsis: true },
          { title: '昵称', dataIndex: 'nickname', ellipsis: true },
          { title: '部门', dataIndex: 'dept_name', ellipsis: true, render: text },
          { title: '绑定时间', dataIndex: 'bind_time', width: 165 },
        ]}
      />

      <div style={{ marginTop: 16 }}>
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 8 }}
          message="后端暂无业务员候选列表接口，请手动输入业务员 userId（正整数，回车确认可多个）；重复绑定幂等。"
        />
        <Select
          mode="tags"
          style={{ width: '100%' }}
          placeholder="输入业务员 userId，按回车添加"
          value={userIds}
          onChange={setUserIds}
          tokenSeparators={[',', ' ']}
          open={false} // 纯输入收集，不提供候选下拉
        />
        {boundIds.length > 0 && (
          <div style={{ marginTop: 8, color: '#999', fontSize: 12 }}>
            当前全量已绑定 userId（参考）：{boundIds.join('、')}
          </div>
        )}
        <div style={{ marginTop: 12, textAlign: 'right' }}>
          <Button style={{ marginRight: 8 }} onClick={onClose}>关 闭</Button>
          <Button type="primary" loading={saving} disabled={userIds.length === 0} onClick={handleBind}>绑定</Button>
        </div>
      </div>
    </Modal>
  )
}

export default BindSalesmanModal
