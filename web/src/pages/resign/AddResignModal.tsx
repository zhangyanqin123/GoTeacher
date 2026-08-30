// 新增离职转移弹窗：原/接收老师下拉（/options 含停用，可搜索），快照后端回查
import { Form, Input, Modal, Select, message } from 'antd'

import { addResign } from '@/api/resign'
import { useTeacherOptions } from '@/hooks/useTeacherOptions'

interface Props {
  open: boolean
  onClose: () => void
  onDone: () => void
}

interface FormValues {
  original_teacher_id: number
  replace_teacher_id: number
  transfer_content: string
}

const AddResignModal = ({ open, onClose, onDone }: Props) => {
  const [form] = Form.useForm<FormValues>()
  const { options, loading } = useTeacherOptions()

  // 下拉 label：姓名（部门）
  const selectOptions = options.map((o) => ({ value: o.id, label: `${o.name}（${o.dept_name}）` }))

  const handleOk = async () => {
    const values = await form.validateFields()
    if (values.original_teacher_id === values.replace_teacher_id) {
      form.setFields([{ name: 'replace_teacher_id', errors: ['接收老师不能与原老师相同'] }])
      return
    }
    try {
      await addResign(values)
      message.success('转移成功')
      form.resetFields()
      onDone()
    } catch {
      // 失败文案已由拦截器统一 message.error，弹窗保留
    }
  }

  return (
    <Modal
      title="新增离职转移"
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      destroyOnHidden
      okText="提 交"
      cancelText="取 消"
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item name="original_teacher_id" label="离职（原）老师" rules={[{ required: true, message: '请选择原老师' }]}>
          <Select placeholder="请选择离职老师（含停用）" showSearch optionFilterProp="label" loading={loading} options={selectOptions} />
        </Form.Item>
        <Form.Item name="replace_teacher_id" label="接收（新）老师" rules={[{ required: true, message: '请选择接收老师' }]}>
          <Select placeholder="请选择接收老师" showSearch optionFilterProp="label" loading={loading} options={selectOptions} />
        </Form.Item>
        <Form.Item name="transfer_content" label="转移内容" rules={[{ required: true, max: 200, message: '请输入转移内容（≤200 字符）' }]}>
          <Input.TextArea placeholder="如：首席投顾" maxLength={200} rows={2} showCount />
        </Form.Item>
      </Form>
    </Modal>
  )
}

export default AddResignModal
