// 审核弹窗（专业/合规，按当前状态定环节）：通过直传换算表；驳回展开富文本原因（必填）后提交
import { useEffect, useState } from 'react'
import { Button, Descriptions, Modal, Space, Tag, message } from 'antd'

import { auditDiagnose, type DiagnoseRow } from '@/api/diagnose'
import { AUDIT_TARGET, auditStageOf } from '@/constants/diagnose'
import RichTextEditor from '@/components/RichTextEditor'
import RichTextView from '@/components/RichTextView'
import { htmlIsEmpty, text } from '@/utils/format'

interface Props {
  row: DiagnoseRow | null
  onClose: () => void
  onDone: () => void
}

const AuditModal = ({ row, onClose, onDone }: Props) => {
  const [rejecting, setRejecting] = useState(false) // 驳回原因编辑态（嵌套展开）
  const [rejectReason, setRejectReason] = useState('')
  const [saving, setSaving] = useState(false)

  const stage = row ? auditStageOf(row.status) : null
  const target = stage ? AUDIT_TARGET[stage] : null

  useEffect(() => {
    setRejecting(false)
    setRejectReason('')
  }, [row])

  const doAudit = async (status: number, rejectReason_: string | undefined) => {
    if (!row) return
    setSaving(true)
    try {
      await auditDiagnose({ id: row.id, status, reject_reason: rejectReason_ })
      message.success(status === target?.pass ? '审核通过' : '已驳回')
      onDone()
    } catch {
      // 失败文案已由拦截器统一 message.error（含非法状态流转）
    } finally {
      setSaving(false)
    }
  }

  const handleReject = async () => {
    if (htmlIsEmpty(rejectReason)) {
      message.warning('驳回原因不能为空')
      return
    }
    if (target) await doAudit(target.reject, rejectReason)
  }

  return (
    <Modal
      title={`审核（ID: ${row?.id ?? '-'}）`}
      open={row !== null}
      onCancel={onClose}
      width={820}
      destroyOnHidden
      footer={
        rejecting ? (
          <Space>
            <Button onClick={() => setRejecting(false)}>返 回</Button>
            <Button danger loading={saving} onClick={handleReject}>确认驳回</Button>
          </Space>
        ) : (
          <Space>
            <Button onClick={onClose}>取 消</Button>
            <Button danger onClick={() => setRejecting(true)}>驳 回</Button>
            <Button type="primary" loading={saving} disabled={!target} onClick={() => target && doAudit(target.pass, undefined)}>
              通 过
            </Button>
          </Space>
        )
      }
    >
      {row && target && stage && (
        <>
          <Descriptions column={2} bordered size="small" style={{ marginTop: 16 }}>
            <Descriptions.Item label="审核类型">
              <Tag color={stage === 'professional' ? 'warning' : 'success'}>{target.title}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="当前状态">{`状态 ${row.status}`}</Descriptions.Item>
            <Descriptions.Item label="用户昵称">{text(row.user_nick_name)}</Descriptions.Item>
            <Descriptions.Item label="用户姓名">{text(row.user_name)}</Descriptions.Item>
            <Descriptions.Item label="股票">{row.stock_code} {row.stock_name}</Descriptions.Item>
            <Descriptions.Item label="买入价 / 持股数">{row.buy_price} / {row.buy_num}</Descriptions.Item>
            <Descriptions.Item label="备注" span={2}><RichTextView html={row.remark} minHeight={24} /></Descriptions.Item>
            <Descriptions.Item label="诊股内容" span={2}><RichTextView html={row.report_content} /></Descriptions.Item>
          </Descriptions>

          {rejecting && (
            <div style={{ marginTop: 16 }}>
              <div style={{ marginBottom: 8, fontWeight: 600, color: '#cf1322' }}>驳回原因（必填，富文本）</div>
              <RichTextEditor value={rejectReason} onChange={setRejectReason} height={220} placeholder="请填写驳回原因..." />
            </div>
          )}
        </>
      )}
    </Modal>
  )
}

export default AuditModal
