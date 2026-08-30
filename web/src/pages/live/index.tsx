// 直播工具调试页（小鹅通透传，公开接口）：① register_user 按手机号幂等注册换 user_id
// ② get_login_url 用该 user_id 换登录链接（有效期 1 分钟，即取即用）——两步顺序依赖
import { useState } from 'react'
import { Alert, Button, Card, Form, Input, Select, Space, Typography, message } from 'antd'
import { CopyOutlined } from '@ant-design/icons'

import { getXeLoginURL, registerXeUser } from '@/api/live'

interface RegisterForm {
  access_token: string
  phone: string
}

interface LoginForm {
  access_token: string
  user_id: string
  login_type: number
  redirect_uri?: string
}

const LOGIN_TYPE_OPTIONS = [
  { value: 1, label: '1 - PC' },
  { value: 2, label: '2 - H5' },
  { value: 3, label: '3 - App' },
]

const LivePage = () => {
  const [registerForm] = Form.useForm<RegisterForm>()
  const [loginForm] = Form.useForm<LoginForm>()
  const [registering, setRegistering] = useState(false)
  const [gettingUrl, setGettingUrl] = useState(false)
  const [loginUrl, setLoginUrl] = useState('')

  // 第一步：注册换 user_id（成功自动带入第二步表单）
  const handleRegister = async (values: RegisterForm) => {
    setRegistering(true)
    try {
      const resp = await registerXeUser(values)
      loginForm.setFieldsValue({ access_token: values.access_token, user_id: resp.user_id })
      message.success(resp.user_exists === 1 ? '用户已存在（幂等注册），user_id 已带入下方' : '注册成功，user_id 已带入下方')
    } catch {
      // 400 形状错 / 502 上游错文案已由拦截器统一 message.error
    } finally {
      setRegistering(false)
    }
  }

  // 第二步：换登录链接（1 分钟有效期）
  const handleGetUrl = async (values: LoginForm) => {
    setGettingUrl(true)
    try {
      const resp = await getXeLoginURL({
        access_token: values.access_token,
        user_id: values.user_id,
        login_type: values.login_type,
        redirect_uri: values.redirect_uri || undefined,
      })
      setLoginUrl(resp.login_url)
    } catch {
      // 文案已弹
    } finally {
      setGettingUrl(false)
    }
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="小鹅通直播透传调试"
        description="凭证 access_token 由小鹅通侧校验（本服务只做形状校验）；进直播间须先按手机号注册换 user_id，再用该 user_id 换登录链接（有效期仅 1 分钟，即取即跳勿缓存）。"
      />

      <Card title="第一步：注册小鹅通用户（幂等）" size="small">
        <Form form={registerForm} layout="vertical" style={{ maxWidth: 520 }} onFinish={handleRegister}>
          <Form.Item name="access_token" label="access_token" rules={[{ required: true, max: 512, message: '必填（≤512 字符）' }]}>
            <Input.Password placeholder="小鹅通 access_token（get_access_token 取得）" visibilityToggle />
          </Form.Item>
          <Form.Item name="phone" label="手机号" rules={[{ required: true, pattern: /^\d{11}$/, message: '请输入 11 位手机号' }]}>
            <Input placeholder="11 位数字" maxLength={11} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={registering}>注册 / 查询 user_id</Button>
        </Form>
      </Card>

      <Card title="第二步：获取登录链接" size="small">
        <Form form={loginForm} layout="vertical" style={{ maxWidth: 520 }} onFinish={handleGetUrl} initialValues={{ login_type: 1 }}>
          <Form.Item name="access_token" label="access_token" rules={[{ required: true, max: 512, message: '必填（≤512 字符）' }]}>
            <Input.Password placeholder="小鹅通 access_token" visibilityToggle />
          </Form.Item>
          <Form.Item name="user_id" label="user_id" rules={[{ required: true, max: 64, message: '必填（商家侧用户唯一标识）' }]}>
            <Input placeholder="可由第一步自动带入" maxLength={64} />
          </Form.Item>
          <Form.Item name="login_type" label="登录类型" rules={[{ required: true }]}>
            <Select options={LOGIN_TYPE_OPTIONS} style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="redirect_uri" label="登录成功跳转链接（可选）" rules={[{ pattern: /^https?:\/\//, message: '必须以 http:// 或 https:// 开头' }]}>
            <Input placeholder="https://...（不传由小鹅通默认跳转）" maxLength={2048} />
          </Form.Item>
          <Space>
            <Button type="primary" htmlType="submit" loading={gettingUrl}>获取登录链接</Button>
          </Space>
        </Form>

        {loginUrl && (
          <div style={{ marginTop: 16, padding: 12, background: '#f6f8fa', borderRadius: 6 }}>
            <Typography.Paragraph copyable={{ text: loginUrl, icon: [<CopyOutlined key="i" />, <span key="t"> 复制链接</span>] }} style={{ marginBottom: 0, wordBreak: 'break-all' }}>
              login_url（1 分钟内有效）：{loginUrl}
            </Typography.Paragraph>
          </div>
        )}
      </Card>
    </Space>
  )
}

export default LivePage
