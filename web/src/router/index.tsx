// 路由表 + RequireAuth 守卫（createBrowserRouter）
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import { Spin } from 'antd'

import { AuthProvider, useAuth } from '@/hooks/useAuth'
import AdminLayout from '@/layouts/AdminLayout'
import LoginPage from '@/pages/login'
import TeacherPage from '@/pages/teacher'
import ResignPage from '@/pages/resign'
import DiagnosePage from '@/pages/diagnose'
import UserPage from '@/pages/user'
import OrderPage from '@/pages/order'
import LivePage from '@/pages/live'
import { getToken } from '@/utils/token'

// 无 token 踢登录（带 redirect 回跳）；token 未 ready（getinfo 重放中）显示 Spin
const RequireAuth = () => {
  const location = useLocation()
  const { ready } = useAuth()
  if (!getToken()) return <Navigate to="/login" replace state={{ redirect: location.pathname + location.search }} />
  if (!ready) return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}><Spin size="large" /></div>
  return <Outlet />
}

const routes = [
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: <RequireAuth />,
    children: [
      {
        element: <AdminLayout />,
        children: [
          { index: true, element: <Navigate to="/teacher" replace /> },
          { path: 'teacher', element: <TeacherPage /> },
          { path: 'resign', element: <ResignPage /> },
          { path: 'diagnose', element: <DiagnosePage /> },
          { path: 'users', element: <UserPage /> },
          { path: 'order', element: <OrderPage /> },
          { path: 'live', element: <LivePage /> },
          { path: '*', element: <Navigate to="/teacher" replace /> },
        ],
      },
    ],
  },
]

const router = createBrowserRouter(routes)

// AuthProvider 需要 Router 上下文（useNavigate），故包在 RouterProvider 外层由 main 组装
export default function AppRoutes() {
  return (
    <AuthProvider>
      <RouterProvider router={router} />
    </AuthProvider>
  )
}
