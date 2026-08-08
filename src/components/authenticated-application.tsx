import App from "@/App"
import { LoginPage } from "@/pages/login"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { useAuth } from "@/lib/auth"
import { StateProvider } from "@/lib/state"

export function AuthenticatedApplication() {
  const { authenticated } = useAuth()
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
