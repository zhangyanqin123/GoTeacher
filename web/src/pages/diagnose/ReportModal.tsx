// 报告弹窗（编写/重新提审，状态 1/3/5）：用户/股票信息 + 备注只读 + 富文本编辑，提交后回落状态 2
import { useEffect, useState } from 'react'
import { Descriptions, Modal, message } from 'antd'

import { submitDiagnoseReport, type DiagnoseRow } from '@/api/diagnose'
import RichTextEditor from '@/components/RichTextEditor'
import RichTextView from '@/components/RichTextView'
import { htmlIsEmpty, text } from '@/utils/format'

interface Props {
  row: DiagnoseRow | null
  onClose: () => void
  onDone: () => void
}

const ReportModal = ({ row, onClose, onDone }: Props) => {
  const [content, setContent] = useState('')
  const [saving, setSaving] = useState(false)

  // 重新提审（3/5）回显已提交的报告内容
  useEffect(() => {
    setContent(row?.report_content ?? '')
  }, [row])

  const handleSubmit = async () => {
    if (!row) return
    if (htmlIsEmpty(content)) {
      message.warning('报告内容不能为空')
      return
    }
    setSaving(true)
    try {
      await submitDiagnoseReport({ id: row.id, report_content: content })
      message.success('提交成功')
      onDone()
    } catch {
      // 失败文案已由拦截器统一 message.error
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={`诊断报告（ID: ${row?.id ?? '-'}）`}
      open={row !== null}
      onOk={handleSubmit}
      onCancel={onClose}
      confirmLoading={saving}
      width={820}
      destroyOnHidden
      okText="提 交"
      cancelText="取 消"
    >
      {row && (
        <>
          <Descriptions column={2} bordered size="small" style={{ marginTop: 16 }}>
            <Descriptions.Item label="用户昵称">{text(row.user_nick_name)}</Descriptions.Item>
            <Descriptions.Item label="用户姓名">{text(row.user_name)}</Descriptions.Item>
            <Descriptions.Item label="股票">{row.stock_code} {row.stock_name}</Descriptions.Item>
            <Descriptions.Item label="买入价 / 持股数">{row.buy_price} / {row.buy_num}</Descriptions.Item>
            <Descriptions.Item label="备注" span={2}><RichTextView html={row.remark} minHeight={24} /></Descriptions.Item>
          </Descriptions>
          <div style={{ margin: '16px 0 8px', fontWeight: 600 }}>诊股内容（富文本）</div>
          <RichTextEditor value={content} onChange={setContent} height={300} placeholder="请编写诊股报告..." />
        </>
      )}
    </Modal>
  )
}

export default ReportModal
