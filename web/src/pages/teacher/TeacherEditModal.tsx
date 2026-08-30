// 编辑老师弹窗：打开时 detail 回显（列名映射契约：rating→level、signature→sign），仅提交 4 个可改字段
import { useEffect, useState } from 'react'
import { Form, Input, Modal, Select, message } from 'antd'

import { getTeacherDetail, updateTeacher } from '@/api/teacher'
import { TEACHER_LEVEL } from '@/constants/teacher'
import { toOptions } from '@/constants/dicts'

interface Props {
  open: boolean
  teacherId: number
  onClose: () => void
  onDone: () => void
}

interface FormValues {
  title: string
  level: number
  avatar: string
  sign: string
}

const TeacherEditModal = ({ open, teacherId, onClose, onDone }: Props) => {
  const [form] = Form.useForm<FormValues>()
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open || !teacherId) return
    setLoading(true)
    form.resetFields()
    getTeacherDetail(teacherId)
      .then((detail) => {
        form.setFieldsValue({ title: detail.title, level: detail.level, avatar: detail.avatar, sign: detail.sign })
      })
      .catch(() => undefined) // 文案已由拦截器弹
      .finally(() => setLoading(false))
  }, [open, teacherId, form])

  const handleOk = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      await updateTeacher({ id: teacherId, title: values.title, level: values.level, avatar: values.avatar, sign: values.sign })
      message.success('编辑成功')
      onDone()
    } catch {
      // 失败保留弹窗（文案已弹）
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={`编辑老师（ID: ${teacherId}）`}
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={saving}
      destroyOnHidden
      okText="保 存"
      cancelText="取 消"
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }} disabled={loading}>
        <Form.Item name="title" label="头衔" rules={[{ required: true, max: 100, message: '请输入头衔（≤100 字符）' }]}>
          <Input placeholder="老师头衔" maxLength={100} />
        </Form.Item>
        <Form.Item name="level" label="评级" rules={[{ required: true }]}>
          <Select placeholder="评级" options={toOptions(TEACHER_LEVEL)} style={{ width: 200 }} />
        </Form.Item>
        <Form.Item name="avatar" label="头像 URL">
          <Input placeholder="头像地址" maxLength={500} />
        </Form.Item>
        <Form.Item name="sign" label="个性签名" rules={[{ max: 200, message: '签名不能超过 200 字符' }]}>
          <Input.TextArea placeholder="个性签名" maxLength={200} rows={3} showCount />
        </Form.Item>
      </Form>
    </Modal>
  )
}

export default TeacherEditModal
