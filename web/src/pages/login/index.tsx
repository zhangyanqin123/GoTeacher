// 登录页：失败文案含「密码」→ 定位密码输入框（后端 ErrInvalidCredentials 措辞契约，勿改写 msg）
import { useState } from 'react'
import { Button, Card, Form, Input } from 'antd'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { useLocation, useNavigate } from 'react-router-dom'

import { login } from '@/api/auth'
import { useAuth } from '@/hooks/useAuth'

interface LoginForm {
  username: string
  password: string
}

const LoginPage = () => {
  const navigate = useNavigate()
  const location = useLocation()
  const { onLoginSuccess } = useAuth()
  const [form] = Form.useForm<LoginForm>()
  const [loading, setLoading] = useState(false)

  // login 特例：失败也 HTTP 200 + code 400，拦截器已 reject ApiError（silent 不弹全局 message）
  const onFinish = async (values: LoginForm) => {
    setLoading(true)
    try {
      const body = await login(values)
      if (body.code === 200 && body.token) {
        onLoginSuccess(body.token, values.username)
        const redirect = (location.state as { redirect?: string } | null)?.redirect
        navigate(redirect && redirect !== '/login' ? redirect : '/teacher', { replace: true })
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : '登录失败'
      // 文案含「密码」→ 错误挂到密码框并聚焦；否则挂到用户名框（对齐旧前端定位约定）
      if (msg.includes('密码')) {
        form.setFields([{ name: 'password', errors: [msg] }])
      } else {
        form.setFields([{ name: 'username', errors: [msg] }])
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #e8f0ff 0%, #f5f7fa 55%)',
      }}
    >
      <Card style={{ width: 380, boxShadow: '0 6px 24px rgba(0, 21, 41, 0.08)', borderRadius: 10 }} styles={{ body: { padding: '12px 8px 8px' } }}>
        <div style={{ textAlign: 'center', marginBottom: 24, fontSize: 20, fontWeight: 600, letterSpacing: 2 }}>股宇宙管理台</div>
        <Form form={form} onFinish={onFinish} size="large" initialValues={{ username: '', password: '' }}>
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="current-password" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" block loading={loading}>
              登 录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}

export default LoginPage
