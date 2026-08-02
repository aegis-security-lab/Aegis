import * as React from "react"
import {
  Blocks,
  Bot,
  CheckCircle2,
  ChevronRight,
  CircleSlash2,
  Globe2,
  MessagesSquare,
  PlugZap,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Wrench,
} from "lucide-react"
import { Link } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { fetchCapabilityCatalog } from "@/lib/api"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  AgentDefinition,
  CapabilityDescriptor,
  CapabilityKind,
} from "@/types"

const kindOrder: CapabilityKind[] = ["tool", "skill", "phone", "web", "mcp"]

const kindMeta: Record<
  string,
  { label: string; description: string; icon: typeof Blocks }
> = {
  tool: {
    label: "控制工具",
    description: "由系统或本地 Source 提供的可调用动作。",
    icon: Wrench,
  },
  skill: {
    label: "Skills",
    description: "作为可信方法和上下文注入一次 Agent 执行。",
    icon: Sparkles,
  },
  phone: {
    label: "Agent Phone",
    description: "每个任务内 Agent 实例一部隔离且持久化的手机。",
    icon: Smartphone,
  },
  web: {
    label: "Web",
    description: "受网络权限约束的公开信息检索能力。",
    icon: Globe2,
  },
  mcp: {
    label: "MCP",
    description: "执行时建立连接并发现工具的外部能力源。",
    icon: PlugZap,
  },
}

export function CapabilitiesPage() {
  const { state } = useAppState()
  const [catalog, setCatalog] = React.useState<CapabilityDescriptor[]>([])
  const [enabled, setEnabled] = React.useState(false)
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState("")

  React.useEffect(() => {
    let active = true
    void fetchCapabilityCatalog()
      .then((result) => {
        if (!active) return
        setCatalog(result.capabilities)
        setEnabled(result.enabled)
        setError("")
      })
      .catch((reason) => {
        if (!active) return
        setError(reason instanceof Error ? reason.message : "读取能力目录失败")
      })
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [])

  const descriptors = React.useMemo(() => {
    const result = [...catalog]
    for (const skill of state?.skills ?? []) {
      if (!result.some((item) => item.kind === "skill" && item.name === skill.id)) {
        result.push({ kind: "skill", name: skill.id })
      }
    }
    return result.sort((left, right) => {
      const leftIndex = kindOrder.indexOf(left.kind)
      const rightIndex = kindOrder.indexOf(right.kind)
      if (leftIndex !== rightIndex) return leftIndex - rightIndex
      return left.name.localeCompare(right.name)
    })
  }, [catalog, state?.skills])

  const agents = (state?.agents ?? []).filter(
    (agent) => agent.enabled && !agent.internal && agent.category !== "concierge"
  )

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Capability control plane"
        title="能力控制台"
        description="管理系统能力、Agent 类型默认分配，以及任务实例运行时的最终授权。"
        actions={
          <Button variant="outline" render={<Link to="/skills" />} nativeButton={false}>
            <Sparkles data-icon="inline-start" />
            管理 Skills
          </Button>
        }
      />

      <AuthorizationRail enabled={enabled} />

      <Tabs defaultValue="catalog" className="gap-5">
        <TabsList variant="line" className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="catalog">能力目录</TabsTrigger>
          <TabsTrigger value="agents">Agent 类型默认能力</TabsTrigger>
          <TabsTrigger value="policy">任务授权模型</TabsTrigger>
        </TabsList>

        <TabsContent value="catalog">
          {loading ? (
            <div className="flex min-h-48 items-center justify-center gap-2 text-sm text-muted-foreground">
              <Spinner />正在读取 Runtime Registry…
            </div>
          ) : error ? (
            <Card>
              <CardContent className="py-10 text-sm text-destructive">{error}</CardContent>
            </Card>
          ) : descriptors.length === 0 ? (
            <Empty className="py-16">
              <EmptyHeader>
                <EmptyMedia variant="icon"><Blocks /></EmptyMedia>
                <EmptyTitle>没有注册能力</EmptyTitle>
                <EmptyDescription>在组合根注册 Capability Source 后会出现在这里。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className="grid gap-4 xl:grid-cols-2">
              {kindOrder.map((kind) => {
                const items = descriptors.filter((item) => item.kind === kind)
                return <CapabilityGroup key={kind} kind={kind} items={items} agents={agents} />
              })}
            </div>
          )}
        </TabsContent>

        <TabsContent value="agents">
          <Card>
            <CardHeader>
              <CardTitle>Agent 类型默认分配</CardTitle>
              <CardDescription>
                这是进入任务前的基线。Task policy 可以进一步收窄，但不能绕过 Runtime Registry。
              </CardDescription>
            </CardHeader>
            <CardContent className="divide-y p-0">
              {agents.map((agent) => (
                <AgentCapabilityRow key={agent.id} agent={agent} webEnabled={Boolean(state?.config.webSearch.enabled)} />
              ))}
              {agents.length === 0 ? (
                <p className="p-6 text-sm text-muted-foreground">暂无启用的 Agent 类型。</p>
              ) : null}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="policy">
          <PolicyModel />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function AuthorizationRail({ enabled }: { enabled: boolean }) {
  const stages = [
    { label: "Runtime Registry", detail: "系统已安装", icon: Blocks },
    { label: "Agent defaults", detail: "Agent 类型默认获得", icon: Bot },
    { label: "Task policy", detail: "任务允许 / 禁止", icon: ShieldCheck },
    { label: "Execution bundle", detail: "本次实际物化", icon: CheckCircle2 },
  ]
  return (
    <Card className="overflow-hidden">
      <CardContent className="p-0">
        <div className="flex items-center justify-between gap-4 border-b bg-muted/25 px-5 py-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <span className={cn("size-2 rounded-full", enabled ? "bg-success" : "bg-destructive")} />
            {enabled ? "能力控制平面在线" : "能力控制平面不可用"}
          </div>
          <span className="font-mono text-[11px] text-muted-foreground">deny by default</span>
        </div>
        <div className="grid md:grid-cols-4">
          {stages.map((stage, index) => (
            <div key={stage.label} className="relative flex min-h-24 items-center gap-3 px-5 py-4 md:border-r md:last:border-r-0">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                <stage.icon className="size-4" />
              </div>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{stage.label}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{stage.detail}</p>
              </div>
              {index < stages.length - 1 ? (
                <ChevronRight className="absolute right-[-9px] z-10 hidden size-4 rounded-full bg-card text-muted-foreground md:block" />
              ) : null}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function CapabilityGroup({ kind, items, agents }: { kind: CapabilityKind; items: CapabilityDescriptor[]; agents: AgentDefinition[] }) {
  const meta = kindMeta[kind] ?? { label: kind, description: "自定义能力来源。", icon: Blocks }
  return (
    <Card className="min-h-64">
      <CardHeader>
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex size-9 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <meta.icon className="size-4" />
            </div>
            <div>
              <CardTitle>{meta.label}</CardTitle>
              <CardDescription className="mt-1">{meta.description}</CardDescription>
            </div>
          </div>
          <Badge variant="outline" className="tabular-nums">{items.length}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {items.map((item) => {
          const usedBy = item.kind === "skill" ? agents.filter((agent) => agent.skillIds.includes(item.name)).length : null
          return (
            <div key={`${item.kind}/${item.name}`} className="flex items-center gap-3 rounded-md bg-muted/35 px-3 py-2.5">
              <code className="min-w-0 flex-1 truncate text-xs">{item.kind}/{item.name}</code>
              {item.dynamic ? <Badge variant="secondary">动态 Source</Badge> : null}
              {usedBy !== null ? <span className="shrink-0 text-xs text-muted-foreground">{usedBy} 个类型</span> : null}
            </div>
          )
        })}
        {items.length === 0 ? (
          <div className="flex min-h-24 flex-col items-center justify-center rounded-md border border-dashed text-center">
            <CircleSlash2 className="mb-2 size-4 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">尚未注册</p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function AgentCapabilityRow({ agent, webEnabled }: { agent: AgentDefinition; webEnabled: boolean }) {
  const nativeCount = agent.skillIds.length + 2 + (webEnabled ? 1 : 0)
  return (
    <div className="flex flex-col gap-4 px-5 py-4 lg:flex-row lg:items-center">
      <div className="flex min-w-56 items-center gap-3">
        <div className="flex size-9 items-center justify-center rounded-md bg-primary/10 text-sm font-semibold text-primary">
          {agent.name.slice(0, 1)}
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{agent.name}</p>
          <p className="truncate font-mono text-[11px] text-muted-foreground">{agent.id}</p>
        </div>
      </div>
      <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
        <Badge variant="secondary"><MessagesSquare className="size-3" />coordination</Badge>
        <Badge variant="secondary"><Smartphone className="size-3" />phone/default</Badge>
        {webEnabled ? <Badge variant="secondary"><Globe2 className="size-3" />web/default</Badge> : null}
        {agent.skillIds.map((id) => <Badge key={id} variant="outline">skill/{id}</Badge>)}
      </div>
      <div className="flex items-center justify-between gap-3 lg:justify-end">
        <span className="text-xs tabular-nums text-muted-foreground">{nativeCount} 项原生能力</span>
        <Button size="sm" variant="ghost" render={<Link to={`/agents?agent=${encodeURIComponent(agent.id)}`} />} nativeButton={false}>配置</Button>
      </div>
    </div>
  )
}

function PolicyModel() {
  const rules = [
    ["inherit", "只使用 Agent 类型默认能力", "最保守"],
    ["merge", "默认能力与任务请求合并", "默认"],
    ["replace", "只采用任务明确请求的能力", "严格隔离"],
  ]
  return (
    <div className="grid gap-4 lg:grid-cols-[1.15fr_.85fr]">
      <Card>
        <CardHeader>
          <CardTitle>任务级授权</CardTitle>
          <CardDescription>策略绑定在任务根节点，整棵 Issue 树共享同一条安全边界。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {rules.map(([name, description, note]) => (
            <div key={name} className="grid gap-2 rounded-md border px-4 py-3 sm:grid-cols-[90px_1fr_auto] sm:items-center">
              <code className="text-xs font-semibold text-primary">{name}</code>
              <span className="text-sm">{description}</span>
              <Badge variant="outline">{note}</Badge>
            </div>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>强制约束</CardTitle>
          <CardDescription>最终结果始终由后端计算，前端不能自行放行。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm">
          <PolicyLine label="Allowed" value="能力白名单；为空时不代表全部允许" />
          <PolicyLine label="Denied" value="显式拒绝，优先级高于默认分配" />
          <PolicyLine label="Required" value="每次执行必须携带的系统能力" />
          <PolicyLine label="Snapshot" value="实际物化结果写入 Execution 证据" />
        </CardContent>
      </Card>
    </div>
  )
}

function PolicyLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-3">
      <ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" />
      <div><p className="font-mono text-xs font-medium">{label}</p><p className="mt-1 text-xs leading-relaxed text-muted-foreground">{value}</p></div>
    </div>
  )
}
