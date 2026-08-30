// localStorage 读写（鉴权双键：token + username）
// hs_username 供用户管理页「不能删当前账号」的客户端禁用判断——getinfo 不返回 id，只能按用户名比对
const TOKEN_KEY = 'hs_token'
const USERNAME_KEY = 'hs_username'

export const getToken = () => localStorage.getItem(TOKEN_KEY) ?? ''
export const getUsername = () => localStorage.getItem(USERNAME_KEY) ?? ''

export const setAuth = (token: string, username: string) => {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USERNAME_KEY, username)
}

export const clearAuth = () => {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USERNAME_KEY)
}
