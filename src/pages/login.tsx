import * as React from "react"
import { KeyRound, LogIn } from "lucide-react"

import { AegisLogo } from "@/components/aegis-logo"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useAuth } from "@/lib/auth"

export function LoginPage() {
  const { login } = useAuth()
  const [password, setPassword] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const [error, setError] = React.useState("")

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!password || busy) return
    setBusy(true)
    setError("")
    try {
      await login(password)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "登录失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-5">
      <section className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center text-center">
          <div className="mb-5 flex size-12 items-center justify-center rounded-xl bg-foreground text-background shadow-sm">
            <AegisLogo className="size-7" />
          </div>
          <h1 className="text-xl font-semibold tracking-tight">打开 Aegis</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            输入服务启动时设置的访问密码
          </p>
        </div>
        <form onSubmit={submit}>
          <FieldGroup>
            <Field data-invalid={Boolean(error) || undefined}>
              <FieldLabel htmlFor="aegis-password">访问密码</FieldLabel>
              <div className="relative">
                <KeyRound className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  id="aegis-password"
                  name="password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  placeholder="输入访问密码…"
                  autoComplete="current-password"
                  className="h-11 pl-10"
                  aria-invalid={Boolean(error) || undefined}
                />
              </div>
              {error ? <FieldError>{error}</FieldError> : null}
            </Field>
            <Button
              className="h-11 w-full"
              type="submit"
              disabled={!password || busy}
            >
              {busy ? <Spinner /> : <LogIn />}
              登录
            </Button>
          </FieldGroup>
        </form>
      </section>
    </main>
  )
}
