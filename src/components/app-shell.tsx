import * as React from "react"
import {
  Building2,
  Boxes,
  CheckSquare2,
  CircleGauge,
  ClipboardCheck,
  FileSearch,
  History,
  ListTodo,
  LibraryBig,
  MessagesSquare,
  MessageCircleMore,
  Moon,
  Plus,
  Radar,
  Search,
  Settings,
  ShieldAlert,
  Sparkles,
  Sun,
  UserRound,
  Users,
  Wrench,
} from "lucide-react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"
import { useTheme } from "next-themes"

import { Button } from "@/components/ui/button"
import { AegisLogo } from "@/components/aegis-logo"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { FloatingTaskPet } from "@/components/floating-task-pet"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"

const ManagementTalentLibrary = React.lazy(() =>
  import("@/pages/talent-library").then((module) => ({
    default: module.TalentLibraryPage,
  }))
)
const ManagementAgents = React.lazy(() =>
  import("@/pages/agents").then((module) => ({ default: module.AgentsPage }))
)
const ManagementDepartments = React.lazy(() =>
  import("@/pages/departments").then((module) => ({
    default: module.DepartmentsPage,
  }))
)
const ManagementSkills = React.lazy(() =>
  import("@/pages/skills").then((module) => ({ default: module.SkillsPage }))
)
const ManagementKnowledgeBases = React.lazy(() =>
  import("@/pages/knowledge-bases").then((module) => ({
    default: module.KnowledgeBasesPage,
  }))
)
const ManagementSessions = React.lazy(() =>
  import("@/pages/sessions").then((module) => ({
    default: module.SessionsPage,
  }))
)
const ManagementApprovals = React.lazy(() =>
  import("@/pages/approvals").then((module) => ({
    default: module.ApprovalsPage,
  }))
)
const ManagementContainers = React.lazy(() =>
  import("@/pages/containers").then((module) => ({
    default: module.ContainersPage,
  }))
)
const ManagementSecurity = React.lazy(() =>
  import("@/pages/security").then((module) => ({
    default: module.SecurityPage,
  }))
)
const ManagementFindings = React.lazy(() =>
  import("@/pages/findings").then((module) => ({
    default: module.FindingsPage,
  }))
)
const ManagementUncover = React.lazy(() =>
  import("@/pages/uncover").then((module) => ({ default: module.UncoverPage }))
)
const ManagementWebSearch = React.lazy(() =>
  import("@/pages/web-search-settings").then((module) => ({
    default: module.WebSearchSettingsPage,
  }))
)
const ManagementSettings = React.lazy(() =>
  import("@/pages/settings").then((module) => ({
    default: module.SettingsPage,
  }))
)

const navigation = [
  {
    label: "工作区",
    items: [
      { to: "/", label: "概览", icon: CircleGauge, end: true },
      { to: "/workspace", label: "管家", icon: Sparkles },
      { to: "/tasks", label: "任务", icon: ListTodo },
      { to: "/timeline", label: "时间线", icon: History },
      { to: "/issues", label: "Board", icon: CheckSquare2 },
      { to: "/relay", label: "Relay", icon: MessageCircleMore },
    ],
  },
]

const managementGroups = [
  {
    label: "Agent 系统",
    items: [
      {
        id: "agents",
        label: "员工管理",
        icon: UserRound,
        page: ManagementAgents,
      },
      {
        id: "talent-library",
        label: "人才库",
        icon: Users,
        page: ManagementTalentLibrary,
      },
      {
        id: "departments",
        label: "组织架构",
        icon: Building2,
        page: ManagementDepartments,
      },
      { id: "skills", label: "Skills", icon: Wrench, page: ManagementSkills },
      {
        id: "knowledge-bases",
        label: "知识库",
        icon: LibraryBig,
        page: ManagementKnowledgeBases,
      },
    ],
  },
  {
    label: "运行时",
    items: [
      {
        id: "sessions",
        label: "Sessions",
        icon: MessagesSquare,
        page: ManagementSessions,
      },
      {
        id: "approvals",
        label: "审批中心",
        icon: ClipboardCheck,
        page: ManagementApprovals,
      },
      {
        id: "containers",
        label: "容器管理",
        icon: Boxes,
        page: ManagementContainers,
      },
    ],
  },
  {
    label: "安全",
    items: [
      {
        id: "security",
        label: "安全态势",
        icon: ShieldAlert,
        page: ManagementSecurity,
      },
      {
        id: "findings",
        label: "发现详情",
        icon: FileSearch,
        page: ManagementFindings,
      },
      {
        id: "uncover",
        label: "空间搜索",
        icon: Radar,
        page: ManagementUncover,
      },
      {
        id: "web-search",
        label: "搜索服务",
        icon: Search,
        page: ManagementWebSearch,
      },
    ],
  },
  {
    label: "设置",
    items: [
      {
        id: "settings",
        label: "全局设置",
        icon: Settings,
        page: ManagementSettings,
      },
    ],
  },
]

export function AppShell() {
  const { error } = useAppState()
  const location = useLocation()
  const { resolvedTheme, setTheme } = useTheme()
  const [managementItem, setManagementItem] = React.useState("talent-library")
  const selectedManagementItem =
    managementGroups
      .flatMap((group) => group.items)
      .find((item) => item.id === managementItem) ??
    managementGroups[0].items[0]
  const ManagementPage = selectedManagementItem.page
  const workspaceRoute =
    location.pathname.startsWith("/workspace") ||
    location.pathname.startsWith("/employees") ||
    location.pathname.startsWith("/relay") ||
    /^\/issues\/[^/]+$/.test(location.pathname)

  return (
    <SidebarProvider
      className="h-svh overflow-hidden"
      style={{ "--sidebar-width": "13.5rem" } as React.CSSProperties}
    >
      <Sidebar collapsible="icon" variant="sidebar">
        <SidebarHeader className="h-14 border-b p-1">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                tooltip="Aegis"
                render={<Link to="/" aria-label="Aegis 首页" />}
              >
                <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
                  <AegisLogo className="size-5" />
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
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarRail />
      </Sidebar>
      <SidebarInset className="h-svh min-h-0 overflow-hidden">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-3 border-b bg-background/90 px-4 backdrop-blur-md lg:px-6">
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
          <Dialog>
            <DialogTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="打开系统管理"
                />
              }
            >
              <Settings />
            </DialogTrigger>
            <DialogContent
              className="grid h-[92dvh] grid-rows-[minmax(0,1fr)] gap-0 overflow-hidden p-0"
              style={{
                width: "min(1500px, calc(100vw - 48px))",
                maxWidth: "none",
              }}
            >
              <DialogHeader className="sr-only">
                <DialogTitle>系统管理</DialogTitle>
                <DialogDescription>
                  在一个窗口中管理 Agent、运行环境、安全能力和全局设置。
                </DialogDescription>
              </DialogHeader>
              <div className="grid size-full min-h-0 grid-cols-[240px_minmax(0,1fr)] overflow-hidden">
                <nav className="min-h-0 overflow-y-auto border-r bg-muted/20 px-3 py-8">
                  {managementGroups.map((group) => (
                    <div key={group.label} className="mb-6 flex flex-col gap-1">
                      <p className="px-3 pb-1 text-xs font-medium text-muted-foreground">
                        {group.label}
                      </p>
                      {group.items.map((item) => (
                        <Button
                          key={item.id}
                          variant={
                            managementItem === item.id ? "secondary" : "ghost"
                          }
                          className="w-full justify-start"
                          onClick={() => setManagementItem(item.id)}
                        >
                          <item.icon data-icon="inline-start" />
                          {item.label}
                        </Button>
                      ))}
                    </div>
                  ))}
                </nav>
                <div className="min-h-0 min-w-0 overflow-y-auto px-6 py-8 lg:px-10">
                  <React.Suspense
                    fallback={
                      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                        正在加载…
                      </div>
                    }
                  >
                    <ManagementPage />
                  </React.Suspense>
                </div>
              </div>
            </DialogContent>
          </Dialog>
        </header>
        <div
          data-slot="app-content-scroller"
          className={cn(
            "min-h-0 w-full flex-1",
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
                : "mx-auto w-full max-w-[1500px] px-4 py-6 lg:px-8 lg:py-8"
            )}
          >
            <Outlet />
          </div>
        </div>
      </SidebarInset>
      <FloatingTaskPet />
    </SidebarProvider>
  )
}
