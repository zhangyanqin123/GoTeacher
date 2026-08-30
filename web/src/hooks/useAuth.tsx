// 鉴权 Context：token 存 localStorage，userInfo 挂载时经 getinfo 重放（F5 保持登录）
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

import { getInfo, logout as apiLogout, type UserInfo } from '@/api/auth'
import { clearAuth, getToken, setAuth } from '@/utils/token'

interface AuthState {
  user: UserInfo | null
  ready: boolean
  onLoginSuccess: (token: string, username: string) => void
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<UserInfo | null>(null)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    if (!getToken()) {
      setReady(true)
      return
    }
    // silent：401/失败由拦截器统一清 token 跳登录，这里不额外弹窗
    getInfo()
      .then((info) => {
        if (info && info.roles.length > 0) setUser(info)
        else clearAuth() // roles 为空视为无效会话
      })
      .catch(() => undefined)
      .finally(() => setReady(true))
  }, [])

  const onLoginSuccess = useCallback((token: string, username: string) => {
    setAuth(token, username)
    getInfo().then((info) => setUser(info)).catch(() => undefined)
  }, [])

  const logout = useCallback(async () => {
    try {
      await apiLogout() // 幂等，失败也继续清本地
    } catch {
      // 静默：本地清理优先
    }
    clearAuth()
    setUser(null)
    window.location.replace('/login') // 整页跳转，彻底清内存态
  }, [])

  return <AuthContext.Provider value={{ user, ready, onLoginSuccess, logout }}>{children}</AuthContext.Provider>
}

export const useAuth = () => {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth 必须在 AuthProvider 内使用')
  return ctx
}
