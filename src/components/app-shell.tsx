import * as React from "react"
import {
  Activity,
  Bot,
  Boxes,
  BriefcaseBusiness,
  Building2,
  CheckSquare2,
  ClipboardCheck,
  FileSearch,
  History,
  LibraryBig,
  ListTodo,
  MessageCircleMore,
  MessagesSquare,
  Moon,
  Plus,
  Radar,
  Settings,
  ShieldAlert,
  Sparkles,
  Sun,
  UserRound,
  Wrench,
  type LucideIcon,
} from "lucide-react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"

import { AegisLogo } from "@/components/aegis-logo"
import { useTheme } from "@/components/theme-provider"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
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
import { cn } from "@/lib/utils"

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

const navigation: NavigationGroup[] = [
  {
    label: "工作",
    items: [
      { to: "/", label: "概览", icon: Activity, end: true },
      { to: "/workspace", label: "管家", icon: Sparkles },
      { to: "/tasks", label: "任务", icon: ListTodo },
      { to: "/issues", label: "计划树", icon: CheckSquare2 },
      { to: "/timeline", label: "时间线", icon: History },
    ],
  },
  {
    label: "团队与能力",
    items: [
      { to: "/employees", label: "员工工作台", icon: UserRound },
      { to: "/relay", label: "团队消息", icon: MessageCircleMore },
      { to: "/agents", label: "员工管理", icon: Bot },
      { to: "/departments", label: "组织架构", icon: Building2 },
      { to: "/talent-library", label: "人才模板", icon: BriefcaseBusiness },
      { to: "/skills", label: "技能", icon: Wrench },
      { to: "/knowledge-bases", label: "知识库", icon: LibraryBig },
    ],
  },
  {
    label: "运行与安全",
    items: [
      {
        to: "/approvals",
        label: "审批",
        icon: ClipboardCheck,
        badge: "approvals",
      },
      { to: "/sessions", label: "执行会话", icon: MessagesSquare },
      { to: "/containers", label: "环境与容器", icon: Boxes },
      { to: "/security", label: "安全态势", icon: ShieldAlert, end: true },
      { to: "/security/findings", label: "安全发现", icon: FileSearch },
      { to: "/security/uncover", label: "网络空间检索", icon: Radar },
    ],
  },
]

const footerNavigation: NavigationItem[] = [
  { to: "/settings", label: "设置", icon: Settings },
]

const allNavigation = [
  ...navigation.flatMap((group) => group.items),
  ...footerNavigation,
]

function isRouteActive(pathname: string, item: NavigationItem) {
  if (item.end) return pathname === item.to
  return pathname === item.to || pathname.startsWith(`${item.to}/`)
}

function currentNavigation(pathname: string) {
  return allNavigation
    .filter((item) => isRouteActive(pathname, item))
    .sort((left, right) => right.to.length - left.to.length)[0]
}

export function AppShell() {
  const { state, error } = useAppState()
  const location = useLocation()
  const { theme, setTheme } = useTheme()
  const activeExecutions =
    state?.executions.filter((execution) =>
      ["queued", "starting", "running", "waiting_approval"].includes(
        execution.status
      )
    ).length ?? 0
  const pendingApprovals =
    state?.approvals.filter((approval) => approval.status === "pending")
      .length ?? 0
  const current = currentNavigation(location.pathname)
  const currentGroup = navigation.find((group) =>
    group.items.some((item) => item.to === current?.to)
  )
  const workspaceRoute =
    location.pathname.startsWith("/workspace") ||
    location.pathname.startsWith("/employees") ||
    location.pathname.startsWith("/relay") ||
    /^\/issues\/[^/]+$/.test(location.pathname)

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
          {navigation.map((group) => (
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
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>

        <SidebarFooter className="border-t border-sidebar-border">
          <SidebarMenu>
            {footerNavigation.map((item) => (
              <SidebarMenuItem key={item.to}>
                <SidebarMenuButton
                  isActive={isRouteActive(location.pathname, item)}
                  tooltip={item.label}
                  render={<NavLink to={item.to} />}
                >
                  <item.icon />
                  <span>{item.label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
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

      <SidebarInset className="h-svh min-h-0 overflow-hidden">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b bg-background/95 px-3 backdrop-blur-sm sm:gap-3 lg:px-5">
          <SidebarTrigger className="size-10 md:size-7" />
          <Separator orientation="vertical" className="h-4" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs text-muted-foreground">
              {currentGroup?.label ?? "Aegis"}
            </p>
            <p className="truncate text-sm font-medium">
              {current?.label ?? "工作区"}
            </p>
          </div>
          {error ? (
            <p
              role="status"
              aria-live="polite"
              className="hidden max-w-60 truncate text-xs text-destructive lg:block"
            >
              {error}
            </p>
          ) : null}
          {activeExecutions > 0 ? (
            <Button
              variant="ghost"
              size="sm"
              className="hidden sm:inline-flex"
              render={<Link to="/sessions" />}
              nativeButton={false}
            >
              <Activity data-icon="inline-start" />
              运行 {activeExecutions}
            </Button>
          ) : null}
          {pendingApprovals > 0 ? (
            <Button
              variant="secondary"
              size="sm"
              render={<Link to="/approvals" />}
              nativeButton={false}
            >
              <ClipboardCheck data-icon="inline-start" />
              <span className="hidden sm:inline">待审批</span>
              <span className="tabular-nums">{pendingApprovals}</span>
            </Button>
          ) : null}
          <Button
            size="sm"
            className="min-h-10 min-w-11 px-3 md:min-h-0 md:min-w-0"
            aria-label="发布任务"
            render={<Link to="/tasks/new" />}
            nativeButton={false}
          >
            <Plus data-icon="inline-start" />
            <span className="hidden sm:inline">发布任务</span>
          </Button>
        </header>

        <div
          id="main-content"
          tabIndex={-1}
          data-slot="app-content-scroller"
          className={cn(
            "min-h-0 w-full flex-1 scroll-mt-14 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
            workspaceRoute
              ? "overflow-hidden"
              : "overflow-y-auto overscroll-y-contain"
          )}
        >
          <div
            data-slot="app-content"
            className={cn(
              workspaceRoute
                ? "flex size-full min-h-0"
                : "mx-auto w-full max-w-[1600px] px-4 py-5 lg:px-6 lg:py-6"
            )}
          >
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
