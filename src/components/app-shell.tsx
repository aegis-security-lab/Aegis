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
import {
  Link,
  NavLink,
  Outlet,
  useLocation,
  useNavigate,
} from "react-router-dom"

import { AegisLogo } from "@/components/aegis-logo"
import { ContentArea } from "@/components/content-area"
import { PageSkeleton } from "@/components/page-skeleton"
import type { OverlayGroup } from "@/components/settings-overlay"
import { SidebarTaskGroup } from "@/components/sidebar-tasks"
import { TabBar, type TabItem } from "@/components/tab-bar"
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
import { lastCrumbLabel } from "@/lib/route-crumbs"
import { cn } from "@/lib/utils"
import {
  loadActiveTabId,
  loadTabs,
  newTabId,
  saveTabs,
  type TabState,
} from "@/lib/tab-storage"
import type { Issue, Task } from "@/types"

const loadSettingsOverlay = () => import("@/components/settings-overlay")
const SettingsOverlay = React.lazy(() =>
  loadSettingsOverlay().then((module) => ({ default: module.SettingsOverlay }))
)

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

function routeTitle(pathname: string, issues: Issue[], tasks: Task[]) {
  return lastCrumbLabel(pathname, issues, tasks) ?? "页面"
}

export function AppShell() {
  const { state, error } = useAppState()
  const location = useLocation()
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()
  const [overlayGroup, setOverlayGroup] = React.useState<OverlayGroup | null>(
    null
  )
  const [tabs, setTabs] = React.useState<TabState[]>(() => {
    const restored = loadTabs()
    if (restored) return restored
    return [{ id: newTabId(), path: location.pathname }]
  })
  const [activeTabId, setActiveTabId] = React.useState<string>(() => {
    const restored = loadActiveTabId()
    if (restored && tabs.some((tab) => tab.id === restored)) return restored
    return tabs[0].id
  })

  React.useEffect(() => {
    saveTabs(tabs, activeTabId)
  }, [tabs, activeTabId])

  const bootedRef = React.useRef(false)
  React.useEffect(() => {
    if (bootedRef.current) return
    bootedRef.current = true
    // Restore the last active tab only on a fresh app open; an explicit
    // deep link (refresh or shared URL) must not be hijacked.
    if (location.pathname !== "/") return
    const active = tabs.find((tab) => tab.id === activeTabId)
    if (active && active.path !== location.pathname) navigate(active.path)
  }, [activeTabId, location.pathname, navigate, tabs])

  React.useEffect(() => {
    // Keep the active tab in sync with links and browser history navigation.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTabs((previous) =>
      previous.map((tab) =>
        tab.id === activeTabId && tab.path !== location.pathname
          ? { ...tab, path: location.pathname }
          : tab
      )
    )
  }, [activeTabId, location.pathname])

  const tabItems: TabItem[] = React.useMemo(
    () =>
      tabs.map((tab) => ({
        ...tab,
        title: routeTitle(tab.path, state?.issues ?? [], state?.tasks ?? []),
      })),
    [state?.issues, state?.tasks, tabs]
  )

  const activateTab = React.useCallback(
    (id: string) => {
      if (id === activeTabId) return
      const tab = tabs.find((candidate) => candidate.id === id)
      if (!tab) return
      setActiveTabId(id)
      navigate(tab.path)
    },
    [activeTabId, navigate, tabs]
  )

  const addTab = React.useCallback(() => {
    const tab: TabState = { id: newTabId(), path: "/workspace" }
    setTabs((previous) => [...previous, tab])
    setActiveTabId(tab.id)
    navigate("/workspace")
  }, [navigate])

  const removeTab = React.useCallback(
    (id: string) => {
      const index = tabs.findIndex((candidate) => candidate.id === id)
      if (index === -1) return
      const next = tabs.filter((candidate) => candidate.id !== id)
      if (next.length === 0) {
        const fresh: TabState = { id: newTabId(), path: "/workspace" }
        setTabs([fresh])
        setActiveTabId(fresh.id)
        navigate("/workspace")
        return
      }
      setTabs(next)
      if (id === activeTabId) {
        const target = next[Math.min(index, next.length - 1)]
        setActiveTabId(target.id)
        navigate(target.path)
      }
    },
    [activeTabId, navigate, tabs]
  )

  const reorderTabs = React.useCallback((fromId: string, toId: string) => {
    if (fromId === toId) return
    setTabs((previous) => {
      const from = previous.findIndex((candidate) => candidate.id === fromId)
      const to = previous.findIndex((candidate) => candidate.id === toId)
      if (from === -1 || to === -1) return previous
      const next = [...previous]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return next
    })
  }, [])
  const pendingApprovals = React.useMemo(
    () =>
      state?.approvals.reduce(
        (count, approval) => count + Number(approval.status === "pending"),
        0
      ) ?? 0,
    [state?.approvals]
  )
  const workspaceRoute =
    location.pathname.startsWith("/workspace") ||
    /^\/issues\/[^/]+$/.test(location.pathname)
  const fullHeightRoute =
    workspaceRoute ||
    (location.pathname !== "/tasks/new" &&
      /^\/tasks\/[^/]+$/.test(location.pathname)) ||
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
        <SidebarHeader className="h-14 p-1.5">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                size="lg"
                tooltip="Aegis"
                render={<Link to="/" aria-label="Aegis 首页" />}
              >
                <span className="relative flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
                  <AegisLogo className="size-5" />
                  <span
                    aria-label={error ? "实时事件正在重连" : "实时事件已连接"}
                    className={cn(
                      "absolute right-0 bottom-0 size-1.5 rounded-full ring-2 ring-primary",
                      error ? "animate-pulse bg-warning" : "bg-success"
                    )}
                  />
                </span>
                <span className="min-w-0 leading-tight">
                  <span className="block truncate text-sm font-semibold tracking-tight">
                    Aegis
                  </span>
                  <span className="block truncate text-[11px] text-muted-foreground">
                    {error ? "Reconnecting…" : "Agent Control Plane"}
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
                  onMouseEnter={() => void loadSettingsOverlay()}
                >
                  <Settings />
                  <span>设置</span>
                  <ChevronsUp className="ml-auto" />
                </DropdownMenuTrigger>
                <DropdownMenuContent side="top" align="start" className="w-64">
                  <DropdownMenuGroup>
                    <DropdownMenuItem onClick={() => setOverlayGroup("agents")}>
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
                    <DropdownMenuItem onClick={() => setOverlayGroup("system")}>
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
                onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              >
                {theme === "dark" ? <Sun /> : <Moon />}
                <span>{theme === "dark" ? "浅色模式" : "深色模式"}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="flex h-svh min-h-0 flex-col p-1.5 sm:p-2">
        <SidebarTrigger className="fixed right-4 bottom-4 z-40 size-11 rounded-full shadow-lg ring-1 ring-foreground/10 md:hidden" />
        <TabBar
          tabs={tabItems}
          activeId={activeTabId}
          onActivate={activateTab}
          onAdd={addTab}
          onRemove={removeTab}
          onReorder={reorderTabs}
        />
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl bg-card shadow-sm ring-1 ring-foreground/5">
          <ContentArea fullHeight={fullHeightRoute}>
            <React.Suspense fallback={<PageSkeleton />}>
              <Outlet />
            </React.Suspense>
          </ContentArea>
        </div>
      </SidebarInset>
      {overlayGroup ? (
        <React.Suspense fallback={null}>
          <SettingsOverlay
            group={overlayGroup}
            open
            onOpenChange={(open) => {
              if (!open) setOverlayGroup(null)
            }}
          />
        </React.Suspense>
      ) : null}
    </SidebarProvider>
  )
}
