/* eslint-disable react-refresh/only-export-components */
import * as React from "react"

import {
  accessToken,
  authorizationHeaders,
  clearAccessToken,
  onUnauthorized,
  saveAccessToken,
} from "@/lib/auth-session"

type AuthValue = {
  authenticated: boolean
  login: (password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = React.createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [authenticated, setAuthenticated] = React.useState(() =>
    Boolean(accessToken())
  )

  React.useEffect(() => {
    const unauthorized = () => setAuthenticated(false)
    return onUnauthorized(unauthorized)
  }, [])

  const login = React.useCallback(async (password: string) => {
    const response = await fetch("/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    })
    const body = (await response.json().catch(() => null)) as {
      token?: string
      error?: string
    } | null
    if (!response.ok || !body?.token) {
      throw new Error(body?.error ?? "登录失败")
    }
    saveAccessToken(body.token)
    setAuthenticated(true)
  }, [])

  const logout = React.useCallback(async () => {
    await fetch("/api/auth/logout", {
      method: "POST",
      headers: authorizationHeaders(),
    }).catch(() => undefined)
    clearAccessToken()
    setAuthenticated(false)
  }, [])

  const value = React.useMemo(
    () => ({ authenticated, login, logout }),
    [authenticated, login, logout]
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = React.useContext(AuthContext)
  if (!value) throw new Error("useAuth must be used inside AuthProvider")
  return value
}
