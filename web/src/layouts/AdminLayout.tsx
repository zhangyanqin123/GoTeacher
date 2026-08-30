// 管理台布局：浅色侧边栏 + 白色 Header（当前页标题 + 用户下拉）+ 灰底内容区
// 整体 100vh 固定：Sider/Header 固定，仅 Content 滚动（避免整页滚动导致的割裂感）
import { useMemo } from 'react'
import { Avatar, Dropdown, Layout, Menu, theme } from 'antd'
import {
  TeamOutlined, UserOutlined, FileTextOutlined, SwapOutlined,
  ShoppingOutlined, VideoCameraOutlined, LogoutOutlined, FundViewOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'

import { useAuth } from '@/hooks/useAuth'

const { Header, Sider, Content } = Layout

// 菜单与路由一一对应（单 admin 角色，固定菜单；不 over-engineer 动态权限）
const MENU_ITEMS = [
  { key: '/teacher', icon: <TeamOutlined />, label: '老师管理' },
  { key: '/resign', icon: <SwapOutlined />, label: '离职转移' },
  { key: '/diagnose', icon: <FileTextOutlined />, label: '诊股记录' },
  { key: '/users', icon: <UserOutlined />, label: '用户管理' },
  { key: '/order', icon: <ShoppingOutlined />, label: '订单管理' },
  { key: '/live', icon: <VideoCameraOutlined />, label: '直播工具' },
]

const AdminLayout = () => {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { user, logout } = useAuth()
  const { token: themeToken } = theme.useToken()

  const activeKey = useMemo(() => `/${pathname.split('/')[1]}`, [pathname])
  const activeLabel = useMemo(() => MENU_ITEMS.find((m) => m.key === activeKey)?.label ?? '', [activeKey])

  return (
    <Layout style={{ height: '100vh', overflow: 'hidden' }}>
      <Sider theme="light" width={216} style={{ borderRight: `1px solid ${themeToken.colorBorderSecondary}`, overflow: 'auto' }}>
        {/* Logo 区 */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10, height: 64 }}>
          <FundViewOutlined style={{ fontSize: 22, color: themeToken.colorPrimary }} />
          <span style={{ fontSize: 16, fontWeight: 600, letterSpacing: 1, color: themeToken.colorTextHeading }}>
            股宇宙管理台
          </span>
        </div>
        <Menu
          theme="light"
          mode="inline"
          selectedKeys={[activeKey]}
          items={MENU_ITEMS}
          onClick={({ key }) => navigate(key)}
          style={{ borderInlineEnd: 'none', padding: '4px 8px' }}
        />
      </Sider>

      <Layout style={{ overflow: 'hidden' }}>
        <Header
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            height: 64,
            paddingInline: 24,
            background: themeToken.colorBgContainer,
            borderBottom: `1px solid ${themeToken.colorBorderSecondary}`,
            lineHeight: 'normal',
          }}
        >
          <span style={{ fontSize: 16, fontWeight: 600, color: themeToken.colorTextHeading }}>{activeLabel}</span>
          <Dropdown
            menu={{
              items: [{ key: 'logout', icon: <LogoutOutlined />, label: '退出登录' }],
              onClick: ({ key }) => key === 'logout' && logout(),
            }}
          >
            <span style={{ cursor: 'pointer', display: 'inline-flex', alignItems: 'center', gap: 8, color: themeToken.colorText }}>
              <Avatar size={28} style={{ background: themeToken.colorPrimary }} icon={<UserOutlined />} />
              {user?.name ?? '-'}
            </span>
          </Dropdown>
        </Header>

        <Content style={{ overflow: 'auto', padding: 20 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

export default AdminLayout
