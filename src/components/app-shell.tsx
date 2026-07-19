import {
  Bot,
  CheckSquare2,
  CircleGauge,
  ClipboardCheck,
  FileSearch,
  History,
  ListTodo,
  LibraryBig,
  MessagesSquare,
  Moon,
  Plus,
  Settings,
  ShieldCheck,
  ShieldAlert,
  Sun,
  Wrench,
} from "lucide-react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"
import { useTheme } from "next-themes"

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

const navigation = [
  {
    label: "工作区",
    items: [
      { to: "/", label: "概览", icon: CircleGauge, end: true },
      { to: "/tasks", label: "任务", icon: ListTodo },
      { to: "/timeline", label: "时间线", icon: History },
      { to: "/issues", label: "Issues", icon: CheckSquare2 },
    ],
  },
  {
    label: "Agent 系统",
    items: [
      { to: "/agents", label: "Agents", icon: Bot },
      { to: "/skills", label: "Skills", icon: Wrench },
      { to: "/knowledge-bases", label: "知识库", icon: LibraryBig },
    ],
  },
  {
    label: "运行时",
    items: [
      { to: "/sessions", label: "Sessions", icon: MessagesSquare },
      { to: "/approvals", label: "审批中心", icon: ClipboardCheck },
    ],
  },
  {
    label: "安全",
    items: [
      { to: "/security", label: "安全态势", icon: ShieldAlert },
      { to: "/security/findings", label: "发现详情", icon: FileSearch },
    ],
  },
]

export function AppShell() {
  const { state, error } = useAppState()
  const location = useLocation()
  const { resolvedTheme, setTheme } = useTheme()
  const pendingApprovals =
    state?.approvals.filter((item) => item.status === "pending").length ?? 0

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon" variant="sidebar">
        <SidebarHeader className="border-b p-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                tooltip="Aegis"
                render={<Link to="/" aria-label="Aegis 首页" />}
              >
                <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
                  <ShieldCheck className="size-4" />
                </span>
                <span className="min-w-0 leading-tight">
                  <span className="block truncate font-semibold">Aegis</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    Pi Control Plane
                  </span>
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          {navigation.map((group) => (
            <SidebarGroup key={group.label}>
              <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {group.items.map((item) => {
                    const active = item.end
                      ? location.pathname === item.to
                      : location.pathname.startsWith(item.to)
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
                        {item.to === "/approvals" && pendingApprovals > 0 ? (
                          <SidebarMenuBadge className="text-amber-700">
                            {pendingApprovals}
                          </SidebarMenuBadge>
                        ) : null}
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarFooter className="border-t p-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={location.pathname.startsWith("/settings")}
                tooltip="设置"
                render={<NavLink to="/settings" />}
              >
                <Settings />
                <span>设置</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b bg-background/90 px-4 backdrop-blur-md lg:px-6">
          <SidebarTrigger />
          <Separator orientation="vertical" className="h-4" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">Aegis Workspace</p>
            {error ? (
              <p className="truncate text-xs text-destructive">{error}</p>
            ) : null}
          </div>
          <Button
            variant="outline"
            size="sm"
            render={<Link to="/tasks/new" />}
            nativeButton={false}
          >
            <Plus />
            <span className="hidden sm:inline">发布任务</span>
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="切换主题"
            onClick={() =>
              setTheme(resolvedTheme === "dark" ? "light" : "dark")
            }
          >
            {resolvedTheme === "dark" ? <Sun /> : <Moon />}
          </Button>
        </header>
        <div className="mx-auto w-full max-w-[1500px] flex-1 px-4 py-6 lg:px-8 lg:py-8">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
