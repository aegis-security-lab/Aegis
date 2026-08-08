import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect, react-refresh/only-export-components */
import { fetchState, subscribeToState } from "@/lib/api"
import type { AppState } from "@/types"
interface Value {
  state: AppState | null
  loading: boolean
  error: string | null
  setState: React.Dispatch<React.SetStateAction<AppState | null>>
  refresh: () => Promise<void>
}
const Context = React.createContext<Value | null>(null)
export function StateProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = React.useState<AppState | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)
  const refresh = React.useCallback(async () => {
    try {
      setState(await fetchState())
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : "无法连接 Aegis")
    } finally {
      setLoading(false)
    }
  }, [])
  React.useEffect(() => {
    void refresh()
    const stop = subscribeToState(
      (v) => {
        React.startTransition(() => {
          setState(v)
          setError(null)
          setLoading(false)
        })
      },
      () => setError("实时事件流正在重连")
    )
    return stop
  }, [refresh])

  const value = React.useMemo(
    () => ({ state, loading, error, setState, refresh }),
    [state, loading, error, refresh]
  )

  return <Context.Provider value={value}>{children}</Context.Provider>
}
export function useAppState() {
  const v = React.useContext(Context)
  if (!v) throw new Error("useAppState must be used inside StateProvider")
  return v
}
