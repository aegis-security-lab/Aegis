import * as React from "react"
import {
  Bot,
  Boxes,
  ClipboardCheck,
  FileSearch,
  FolderCog,
  Gauge,
  Languages,
  LibraryBig,
  MessagesSquare,
  Radar,
  Search,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Wrench,
  X,
  type LucideIcon,
} from "lucide-react"

import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import type { SettingsSection } from "@/pages/settings"

// Lazy-loaded pages so each overlay group only pays for what it shows.
const AgentsPage = React.lazy(() =>
  import("@/pages/agents").then((m) => ({ default: m.AgentsPage }))
)
const CapabilitiesPage = React.lazy(() =>
  import("@/pages/capabilities").then((m) => ({ default: m.CapabilitiesPage }))
)
const SkillsPage = React.lazy(() =>
  import("@/pages/skills").then((m) => ({ default: m.SkillsPage }))
)
const KnowledgeBasesPage = React.lazy(() =>
  import("@/pages/knowledge-bases").then((m) => ({
    default: m.KnowledgeBasesPage,
  }))
)
const ApprovalsPage = React.lazy(() =>
  import("@/pages/approvals").then((m) => ({ default: m.ApprovalsPage }))
)
const SessionsPage = React.lazy(() =>
  import("@/pages/sessions").then((m) => ({ default: m.SessionsPage }))
)
const ContainersPage = React.lazy(() =>
  import("@/pages/containers").then((m) => ({ default: m.ContainersPage }))
)
const SecurityPage = React.lazy(() =>
  import("@/pages/security").then((m) => ({ default: m.SecurityPage }))
)
const FindingsPage = React.lazy(() =>
  import("@/pages/findings").then((m) => ({ default: m.FindingsPage }))
)
const UncoverPage = React.lazy(() =>
  import("@/pages/uncover").then((m) => ({ default: m.UncoverPage }))
)
const SettingsPage = React.lazy(() =>
  import("@/pages/settings").then((m) => ({ default: m.SettingsPage }))
)

export type OverlayGroup = "agents" | "runtime" | "system"

type NavItem = {
  key: string
  label: string
  icon: LucideIcon
}

type GroupConfig = {
  label: string
  items: NavItem[]
}

const GROUP_CONFIG: Record<OverlayGroup, GroupConfig> = {
  agents: {
    label: "Agent 与能力",
    items: [
      { key: "agents", label: "Agent 类型", icon: Bot },
      { key: "capabilities", label: "能力控制台", icon: Wrench },
      { key: "skills", label: "Skills", icon: Sparkles },
      { key: "knowledge-bases", label: "知识库", icon: LibraryBig },
    ],
  },
  runtime: {
    label: "运行与安全",
    items: [
      { key: "approvals", label: "审批", icon: ClipboardCheck },
      { key: "sessions", label: "执行会话", icon: MessagesSquare },
      { key: "containers", label: "环境与容器", icon: Boxes },
      { key: "security", label: "安全态势", icon: ShieldAlert },
      { key: "security/findings", label: "安全发现", icon: FileSearch },
      { key: "security/uncover", label: "网络空间检索", icon: Radar },
    ],
  },
  system: {
    label: "系统设置",
    items: [
      { key: "general", label: "全局配置", icon: Languages },
      { key: "runtime", label: "AgentCore", icon: Radar },
      { key: "model", label: "模型与认证", icon: Bot },
      { key: "search", label: "Web 搜索", icon: Search },
      { key: "workspace", label: "工作区", icon: FolderCog },
      { key: "budget", label: "预算与心跳", icon: Gauge },
      { key: "policy", label: "执行与拆分策略", icon: ShieldCheck },
    ],
  },
}

function renderOverlayPage(item: string): React.ReactNode {
  switch (item) {
    case "agents":
      return <AgentsPage />
    case "capabilities":
      return <CapabilitiesPage />
    case "skills":
      return <SkillsPage />
    case "knowledge-bases":
      return <KnowledgeBasesPage />
    case "approvals":
      return <ApprovalsPage />
    case "sessions":
      return <SessionsPage />
    case "containers":
      return <ContainersPage />
    case "security":
      return <SecurityPage />
    case "security/findings":
      return <FindingsPage />
    case "security/uncover":
      return <UncoverPage />
    default:
      return null
  }
}

function OverlayFallback() {
  return (
    <div className="flex h-64 items-center justify-center gap-2 text-sm text-muted-foreground">
      <Spinner />
      加载中…
    </div>
  )
}

export function SettingsOverlay({
  group,
  open,
  onOpenChange,
}: {
  group: OverlayGroup
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const config = GROUP_CONFIG[group]
  const [active, setActive] = React.useState(() => ({
    group,
    item: GROUP_CONFIG[group].items[0].key,
  }))
  const activeItem = active.group === group ? active.item : config.items[0].key
  const currentItem =
    config.items.find((item) => item.key === activeItem) ?? config.items[0]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[calc(100svh-2rem)] w-[calc(100vw-4rem)] flex-row gap-0 overflow-hidden p-0 sm:max-w-[1400px]"
      >
        <DialogTitle className="sr-only">{config.label}</DialogTitle>
        <DialogDescription className="sr-only">
          浏览并管理{config.label}
        </DialogDescription>

        <aside className="hidden w-56 shrink-0 flex-col border-r bg-sidebar/50 md:flex">
          <div className="flex h-14 items-center border-b px-4">
            <span className="truncate text-sm font-semibold">
              {config.label}
            </span>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <nav
              className="flex flex-col gap-0.5 p-2"
              aria-label={config.label}
            >
              {config.items.map((item) => {
                const Icon = item.icon
                const active = activeItem === item.key
                return (
                  <Button
                    key={item.key}
                    type="button"
                    variant={active ? "secondary" : "ghost"}
                    className="justify-start"
                    onClick={() => setActive({ group, item: item.key })}
                    aria-current={active ? "page" : undefined}
                  >
                    <Icon data-icon="inline-start" />
                    <span>{item.label}</span>
                  </Button>
                )
              })}
            </nav>
          </ScrollArea>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col bg-background">
          <div className="flex h-14 shrink-0 items-center justify-between border-b px-4">
            <h2 className="truncate text-sm font-medium">
              {currentItem.label}
            </h2>
            <DialogClose
              render={<Button variant="ghost" size="icon-sm" />}
              aria-label="关闭"
            >
              <X />
              <span className="sr-only">关闭</span>
            </DialogClose>
          </div>
          <nav
            className="flex shrink-0 gap-1 overflow-x-auto border-b px-3 py-2 md:hidden"
            aria-label={config.label}
          >
            {config.items.map((item) => {
              const Icon = item.icon
              const active = activeItem === item.key
              return (
                <Button
                  key={item.key}
                  type="button"
                  variant={active ? "secondary" : "ghost"}
                  size="sm"
                  className="shrink-0"
                  onClick={() => setActive({ group, item: item.key })}
                  aria-current={active ? "page" : undefined}
                >
                  <Icon data-icon="inline-start" />
                  <span>{item.label}</span>
                </Button>
              )
            })}
          </nav>
          <ScrollArea className="min-h-0 flex-1">
            <div className="mx-auto w-full max-w-[1200px] px-4 py-5 lg:px-6 lg:py-6 [&_[data-slot=page-header-identity]]:hidden [&_[data-slot=page-header]]:justify-end [&_[data-slot=page-header]]:border-0 [&_[data-slot=page-header]]:pb-0">
              <React.Suspense fallback={<OverlayFallback />}>
                {group === "system" ? (
                  <SettingsPage
                    embedded
                    section={activeItem as SettingsSection}
                    onSectionChange={(section) =>
                      setActive({ group, item: section })
                    }
                  />
                ) : (
                  renderOverlayPage(activeItem)
                )}
              </React.Suspense>
            </div>
          </ScrollArea>
        </div>
      </DialogContent>
    </Dialog>
  )
}
