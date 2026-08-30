// 诊股详情 Drawer：全字段 + 报告富文本 + 审核流程日志（audit_logs）
import { Drawer, Descriptions, Table, Tag } from 'antd'
import { useEffect, useState } from 'react'

import { getDiagnoseDetail, type DiagnoseAuditLog, type DiagnoseDetail, type DiagnoseRow } from '@/api/diagnose'
import { DIAGNOSE_STATUS } from '@/constants/diagnose'
import StatusTag from '@/components/StatusTag'
import RichTextView from '@/components/RichTextView'
import { text } from '@/utils/format'

const DetailDrawer = ({ row, onClose }: { row: DiagnoseRow | null; onClose: () => void }) => {
  const [detail, setDetail] = useState<DiagnoseDetail | null>(null)

  useEffect(() => {
    setDetail(null)
    if (!row) return
    getDiagnoseDetail(row.id)
      .then(setDetail)
      .catch(() => undefined)
  }, [row])

  const d = detail ?? row // 加载中先展示列表行冗余字段

  return (
    <Drawer title={`诊股详情（ID: ${row?.id ?? '-'}）`} open={row !== null} onClose={onClose} width={640}>
      {d && (
        <>
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="用户昵称">{text(d.user_nick_name)}</Descriptions.Item>
            <Descriptions.Item label="用户姓名">{text(d.user_name)}</Descriptions.Item>
            <Descriptions.Item label="股票代码">{d.stock_code}</Descriptions.Item>
            <Descriptions.Item label="股票名称">{d.stock_name}</Descriptions.Item>
            <Descriptions.Item label="买入价">{d.buy_price}</Descriptions.Item>
            <Descriptions.Item label="持股数">{d.buy_num}</Descriptions.Item>
            <Descriptions.Item label="老师">{text(d.teacher_name)}</Descriptions.Item>
            <Descriptions.Item label="状态"><StatusTag dict={DIAGNOSE_STATUS} value={d.status} /></Descriptions.Item>
            <Descriptions.Item label="提交时间">{text(d.submit_time)}</Descriptions.Item>
            <Descriptions.Item label="报告提交时间">{text(d.report_submit_time)}</Descriptions.Item>
            <Descriptions.Item label="备注" span={2}><RichTextView html={d.remark} minHeight={24} /></Descriptions.Item>
            <Descriptions.Item label="诊股报告" span={2}><RichTextView html={d.report_content} /></Descriptions.Item>
          </Descriptions>

          <div style={{ margin: '16px 0 8px', fontWeight: 600 }}>审核流程日志</div>
          <Table<DiagnoseAuditLog>
            rowKey={(r) => `${r.time}-${r.type}-${r.operator}`}
            size="small"
            bordered
            dataSource={detail?.audit_logs ?? []}
            pagination={false}
            columns={[
              { title: '时间', dataIndex: 'time', width: 165 },
              { title: '类型', dataIndex: 'type', width: 90, render: (v: string) => <Tag>{v}</Tag> },
              { title: '操作人', dataIndex: 'operator', width: 90 },
              { title: '结果', dataIndex: 'result', width: 80 },
              { title: '备注', dataIndex: 'remark', ellipsis: true, render: (v: string) => (v ? <RichTextView html={v} minHeight={20} /> : '-') },
            ]}
          />
        </>
      )}
    </Drawer>
  )
}

export default DetailDrawer
