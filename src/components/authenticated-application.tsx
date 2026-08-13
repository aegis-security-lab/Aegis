import App from "@/App"
import { LoginPage } from "@/pages/login"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { Spinner } from "@/components/ui/spinner"
import { useAuth } from "@/lib/auth"
import { StateProvider } from "@/lib/state"

export function AuthenticatedApplication() {
  const { authenticated } = useAuth()
  if (authenticated === null) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background">
        <Spinner aria-label="正在验证登录状态" />
      </main>
    )
  }
  if (!authenticated) return <LoginPage />
  return (
    <StateProvider>
      <TooltipProvider>
        <App />
        <Toaster position="top-right" richColors />
      </TooltipProvider>
    </StateProvider>
  )
}
