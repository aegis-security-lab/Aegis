import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"

import "./index.css"
import App from "./App.tsx"
import { ThemeProvider } from "@/components/theme-provider.tsx"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { StateProvider } from "@/lib/state"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="light" storageKey="aegis-theme">
      <BrowserRouter>
        <StateProvider>
          <TooltipProvider>
            <App />
            <Toaster position="top-right" richColors />
          </TooltipProvider>
        </StateProvider>
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>
)
