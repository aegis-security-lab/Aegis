import * as React from "react"
import {
  Activity,
  Bot,
  ChevronsUp,
  Moon,
  Settings,
  ShieldAlert,
  Sparkles,
  Sun,
  type LucideIcon,
} from "lucide-react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"

import { AegisLogo } from "@/components/aegis-logo"
import { ContentArea } from "@/components/content-area"
import { SettingsOverlay, type OverlayGroup } from "@/components/settings-overlay"
import { SidebarTaskGroup } from "@/components/sidebar-tasks"
import { useTheme } from "@/components/theme-provider"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { useAppState } from "@/lib/state"

type NavigationItem = {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
  badge?: "approvals"
}

type NavigationGroup = {
  label: string
  items: NavigationItem[]
}

const workNavigation: NavigationGroup[] = [
  {
    label: "工作",
    items: [
      { to: "/", label: "概览", icon: Activity, end: true },
      { to: "/workspace", label: "管家", icon: Sparkles },
    ],
  },
]

function isRouteActive(pathname: string, item: NavigationItem) {
  if (item.end) return pathname === item.to
  return pathname === item.to || pathname.startsWith(`${item.to}/`)
}

export function AppShell() {
  const { state } = useAppState()
  const location = useLocation()
  const { theme, setTheme } = useTheme()
  const [overlayGroup, setOverlayGroup] = React.useState<OverlayGroup | null>(
    null
  )
  const pendingApprovals =
    state?.approvals.filter((approval) => approval.status === "pending")
      .length ?? 0
  const workspaceRoute =
    location.pathname.startsWith("/workspace") ||
    /^\/issues\/[^/]+$/.test(location.pathname)
  const fullHeightRoute =
    workspaceRoute ||
    /^\/tasks\/[^/]+$/.test(location.pathname) ||
    /^\/tasks\/[^/]+\/(issues|board)$/.test(location.pathname)

  return (
    <SidebarProvider
      className="h-svh overflow-hidden"
      style={{ "--sidebar-width": "15rem" } as React.CSSProperties}
    >
      <a
        href="#main-content"
        className="fixed top-2 left-2 z-50 -translate-y-16 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-transform focus:translate-y-0"
      >
        跳到主要内容
      </a>
      <Sidebar collapsible="icon" variant="sidebar">
        <SidebarHeader className="h-14 border-b border-sidebar-border p-1.5">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                tooltip="Aegis"
                render={<Link to="/" aria-label="Aegis 首页" />}
              >
                <span className="relative flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
                  <AegisLogo className="size-5" />
                  <span className="absolute right-0 bottom-0 size-1.5 rounded-full bg-emerald-400 ring-2 ring-primary" />
                </span>
                <span className="min-w-0 leading-tight">
                  <span className="block truncate text-sm font-semibold tracking-tight">
                    Aegis
                  </span>
                  <span className="block truncate text-[11px] text-muted-foreground">
                    Agent Control Plane
                  </span>
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>

        <SidebarContent aria-label="主导航">
          {workNavigation.map((group) => (
            <SidebarGroup key={group.label}>
              <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {group.items.map((item) => {
                    const active = isRouteActive(location.pathname, item)
                    const badge =
                      item.badge === "approvals" ? pendingApprovals : undefined
                    return (
                      <SidebarMenuItem key={item.to}>
                        <SidebarMenuButton
                          isActive={active}
                          tooltip={item.label}
                          render={<NavLink to={item.to} />}
                        >
                          <item.icon />
                          <span>{item.label}</span>
                        </SidebarMenuButton>
                        {badge ? (
                          <SidebarMenuBadge>{badge}</SidebarMenuBadge>
                        ) : null}
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
                {group.label === "工作" ? <SidebarTaskGroup /> : null}
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>

        <SidebarFooter className="border-t border-sidebar-border">
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={<SidebarMenuButton tooltip="设置" />}
                  openOnHover
                  closeDelay={150}
                >
                  <Settings />
                  <span>设置</span>
                  <ChevronsUp className="ml-auto" />
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  side="top"
                  align="start"
                  className="w-64"
                >
                  <DropdownMenuGroup>
                    <DropdownMenuItem
                      onClick={() => setOverlayGroup("agents")}
                    >
                      <Bot data-icon="inline-start" />
                      Agent 与能力
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => setOverlayGroup("runtime")}
                    >
                      <ShieldAlert data-icon="inline-start" />
                      运行与安全
                      {pendingApprovals > 0 ? (
                        <span className="ml-auto rounded-md bg-muted px-1.5 text-xs tabular-nums">
                          {pendingApprovals}
                        </span>
                      ) : null}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => setOverlayGroup("system")}
                    >
                      <Settings data-icon="inline-start" />
                      系统设置
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={theme === "dark" ? "切换到浅色" : "切换到深色"}
                onClick={() =>
                  setTheme(theme === "dark" ? "light" : "dark")
                }
              >
                {theme === "dark" ? <Sun /> : <Moon />}
                <span>
                  {theme === "dark" ? "浅色模式" : "深色模式"}
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="flex h-svh min-h-0 flex-col p-1.5 sm:p-2">
        <SidebarTrigger className="fixed right-4 bottom-4 z-40 size-11 rounded-full shadow-lg ring-1 ring-foreground/10 md:hidden" />
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl bg-card shadow-sm ring-1 ring-foreground/5">
          <ContentArea fullHeight={fullHeightRoute}>
            <Outlet />
          </ContentArea>
        </div>
      </SidebarInset>
      <SettingsOverlay
        group={overlayGroup ?? "agents"}
        open={overlayGroup !== null}
        onOpenChange={(open) => {
          if (!open) setOverlayGroup(null)
        }}
      />
    </SidebarProvider>
  )
}
