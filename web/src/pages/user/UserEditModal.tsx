// 新增/编辑用户弹窗（合一）：编辑时 password 留空=不修改（后端契约）
import { useEffect } from 'react'
import { Form, Input, Modal, message } from 'antd'

import { addAdminUser, editAdminUser, type AdminUserRow } from '@/api/adminUser'

interface Props {
  open: boolean
  record: AdminUserRow | null // null=新增
  onClose: () => void
  onDone: () => void
}

interface FormValues {
  username: string
  password: string
}

const UserEditModal = ({ open, record, onClose, onDone }: Props) => {
  const [form] = Form.useForm<FormValues>()
  const isEdit = record !== null

  useEffect(() => {
    if (open) {
      form.resetFields()
      form.setFieldsValue({ username: record?.username ?? '', password: '' })
    }
  }, [open, record, form])

  const handleOk = async () => {
    const values = await form.validateFields()
    try {
      if (isEdit && record) {
        await editAdminUser({ id: record.id, username: values.username, password: values.password })
        message.success('编辑成功')
      } else {
        await addAdminUser({ username: values.username, password: values.password })
        message.success('新增成功')
      }
      onDone()
    } catch {
      // 失败文案由拦截器统一 message.error（用户名已存在等），弹窗保留供修改
    }
  }

  return (
    <Modal
      title={isEdit ? '编辑用户' : '新增用户'}
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      destroyOnHidden
      okText="保 存"
      cancelText="取 消"
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item name="username" label="用户名" rules={[{ required: true, max: 50, message: '请输入用户名（≤50 字符）' }]}>
          <Input placeholder="请输入用户名" maxLength={50} />
        </Form.Item>
        <Form.Item
          name="password"
          label="密码"
          rules={[
            { required: !isEdit, message: '请输入密码' },
            { min: 6, max: 64, message: '密码长度 6-64 位' },
          ]}
        >
          <Input.Password placeholder={isEdit ? '留空则不修改密码' : '请输入密码（6-64 位）'} maxLength={64} autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

export default UserEditModal
