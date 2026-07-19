import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import { useVirtualizer } from "@tanstack/react-virtual"
import {
  ArrowLeft,
  Bot,
  CircleCheckBig,
  ChevronDown,
  ChevronUp,
  Clock3,
  Coins,
  ExternalLink,
  FileText,
  ListChecks,
  MessagesSquare,
  ScrollText,
  SquareTerminal,
  Wrench,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"

import { ExecutionEvents } from "@/components/execution-events"
import { MarkdownContent } from "@/components/markdown-content"
import { PageHeader } from "@/components/page-header"
import { SessionConversation } from "@/components/session-conversation"
import { StatusBadge } from "@/components/status-badge"
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
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  fetchSessionDelta,
  fetchSessionDetail,
  fetchSessionEvents,
  fetchSessionMessages,
  fetchSessionProgress,
} from "@/lib/api"
import {
  chronological,
  mergeById,
  reverseChronological,
} from "@/lib/collections"
import {
  formatCost,
  formatDuration,
  formatTime,
  formatTokens,
} from "@/lib/format"
import { useAppState } from "@/lib/state"
import type {
  SessionDetail,
  ExecutionProgress,
  ToolParameterSnapshot,
  ToolSnapshot,
} from "@/types"

export function SessionDetailPage() {
  const { executionId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<SessionDetail | null>(null)
  const [error, setError] = React.useState<string | null>(null)
  const [loadingMore, setLoadingMore] = React.useState<
    "messages" | "events" | "progress" | null
  >(null)
  const watermarkRef = React.useRef("")

  const load = React.useCallback(async () => {
    if (!executionId) return
    try {
      const next = await fetchSessionDetail(executionId)
      watermarkRef.current = next.watermark
      setDetail(next)
      setError(null)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Session 读取失败")
    }
  }, [executionId])

  React.useEffect(() => {
    void load()
  }, [load])

  const live = detail
    ? ["queued", "starting", "running", "waiting_approval"].includes(
        detail.session.execution.status
      )
    : false
  React.useEffect(() => {
    if (!executionId || !live || !watermarkRef.current) return
    const timer = window.setTimeout(() => {
      void fetchSessionDelta(executionId, watermarkRef.current)
        .then((delta) => {
          watermarkRef.current = delta.watermark
          setDetail((current) => {
            if (!current) return current
            const newMessageCount = delta.messages.filter(
              (item) =>
                !current.messages.some((existing) => existing.id === item.id)
            ).length
            const newEventCount = delta.events.filter(
              (item) =>
                !current.events.some((existing) => existing.id === item.id)
            ).length
            const newProgressCount = delta.progressUpdates.filter(
              (item) =>
                !current.progressUpdates.some(
                  (existing) => existing.id === item.id
                )
            ).length
            return {
              ...current,
              session: { ...current.session, execution: delta.execution },
              messages: mergeById(
                current.messages,
                delta.messages,
                chronological
              ),
              events: mergeById(
                current.events,
                delta.events,
                reverseChronological
              ),
              progressUpdates: mergeById(
                current.progressUpdates,
                delta.progressUpdates,
                chronological
              ),
              messagesPage: {
                ...current.messagesPage,
                total: current.messagesPage.total + newMessageCount,
              },
              eventsPage: {
                ...current.eventsPage,
                total: current.eventsPage.total + newEventCount,
              },
              progressPage: {
                ...current.progressPage,
                total: current.progressPage.total + newProgressCount,
              },
              watermark: delta.watermark,
            }
          })
        })
        .catch(() => void load())
    }, 250)
    return () => window.clearTimeout(timer)
  }, [executionId, live, load, state?.updatedAt])

  const loadOlder = async (kind: "messages" | "events" | "progress") => {
    if (!executionId || !detail || loadingMore) return
    setLoadingMore(kind)
    try {
      if (kind === "messages") {
        const page = await fetchSessionMessages(
          executionId,
          detail.messagesPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                messages: mergeById(
                  current.messages,
                  page.items,
                  chronological
                ),
                messagesPage: page.page,
              }
            : current
        )
      } else if (kind === "events") {
        const page = await fetchSessionEvents(
          executionId,
          detail.eventsPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                events: mergeById(
                  current.events,
                  page.items,
                  reverseChronological
                ),
                eventsPage: page.page,
              }
            : current
        )
      } else {
        const page = await fetchSessionProgress(
          executionId,
          detail.progressPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                progressUpdates: mergeById(
                  current.progressUpdates,
                  page.items,
                  chronological
                ),
                progressPage: page.page,
              }
            : current
        )
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "读取历史记录失败")
    } finally {
      setLoadingMore(null)
    }
  }

  if (error && !detail) {
    return (
      <Empty className="min-h-96">
        <EmptyHeader>
          <EmptyTitle>无法打开 Session</EmptyTitle>
          <EmptyDescription>{error}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant="outline"
            render={<Link to="/sessions" />}
            nativeButton={false}
          >
            <ArrowLeft data-icon="inline-start" />
            返回 Sessions
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  if (!detail) {
    return (
      <div className="flex min-h-96 items-center justify-center">
        <Spinner className="size-5 text-muted-foreground" />
      </div>
    )
  }

  const { execution } = detail.session
  const initialPrompt = execution.initialPrompt ?? ""
  const systemPrompt = execution.systemPrompt ?? ""
  const pricing = execution.pricing
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={`${detail.session.issueIdentifier} · Session detail`}
        title={detail.session.agentName}
        description={execution.sessionId}
        actions={
          <>
            <Button
              variant="ghost"
              render={<Link to="/sessions" />}
              nativeButton={false}
            >
              <ArrowLeft data-icon="inline-start" />
              返回
            </Button>
            <Button
              variant="outline"
              render={<Link to={`/issues/${execution.issueId}`} />}
              nativeButton={false}
            >
              <ExternalLink data-icon="inline-start" />
              查看 Issue
            </Button>
          </>
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge status={execution.status} />
        <Badge variant="secondary">{execution.kind}</Badge>
        <Badge variant="outline">
          {execution.provider} / {execution.model}
        </Badge>
        <Badge variant="outline">{formatTokens(execution.tokens)} tokens</Badge>
        <Badge variant="outline">{formatCost(execution.cost)}</Badge>
      </div>

      <Tabs defaultValue="conversation" className="min-h-0">
        <TabsList variant="line">
          <TabsTrigger value="conversation">
            <MessagesSquare />
            对话 {detail.messagesPage.total}
          </TabsTrigger>
          <TabsTrigger value="events">
            <SquareTerminal />
            事件 {detail.eventsPage.total}
          </TabsTrigger>
          <TabsTrigger value="progress">
            <ListChecks />
            进度 {detail.progressPage.total}
          </TabsTrigger>
          <TabsTrigger value="prompts">
            <ScrollText />
            提示词
          </TabsTrigger>
          <TabsTrigger value="tools">
            <Wrench />
            工具 {execution.toolsSnapshot?.length ?? 0}
          </TabsTrigger>
          <TabsTrigger value="metadata">运行信息</TabsTrigger>
        </TabsList>

        <TabsContent value="conversation" className="pt-4">
          <Card className="h-[calc(100svh-17rem)] min-h-[520px] gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle className="flex min-w-0 items-center gap-2">
                <Bot className="size-4 shrink-0" />
                <span className="truncate" title={detail.session.issueTitle}>
                  {detail.session.issueTitle}
                </span>
              </CardTitle>
              <CardDescription>
                这里展示该 Pi Session 中实际持久化的用户与 AI 消息。
              </CardDescription>
            </CardHeader>
            <CardContent className="min-h-0 flex-1 p-0">
              <SessionConversation
                messages={detail.messages}
                agentName={detail.session.agentName}
                issueIdentifier={detail.session.issueIdentifier}
                initialPrompt={initialPrompt}
                hasMore={detail.messagesPage.hasMore}
                loadingMore={loadingMore === "messages"}
                onLoadMore={() => void loadOlder("messages")}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events" className="pt-4">
          <ExecutionEvents
            events={detail.events}
            issues={state?.issues ?? []}
            hasMore={detail.eventsPage.hasMore}
            loadingMore={loadingMore === "events"}
            onLoadMore={() => void loadOlder("events")}
            className="h-[calc(100svh-17rem)] min-h-[520px]"
          />
        </TabsContent>

        <TabsContent value="progress" className="pt-4">
          <WorkProgressPanel
            updates={detail.progressUpdates}
            hasMore={detail.progressPage.hasMore}
            loadingMore={loadingMore === "progress"}
            onLoadMore={() => void loadOlder("progress")}
          />
        </TabsContent>

        <TabsContent value="prompts" className="pt-4">
          <div className="grid gap-4 xl:grid-cols-2">
            <PromptCard
              icon={FileText}
              title="启动任务提示词"
              description="创建 Session 时发送给 Pi 的第一条任务 Prompt。"
              content={initialPrompt}
            />
            <PromptCard
              icon={ScrollText}
              title="Agent 系统提示词"
              description="该 Execution 启动时冻结的 Agent System Prompt 快照。"
              content={systemPrompt}
            />
          </div>
        </TabsContent>

        <TabsContent value="tools" className="pt-4">
          <ToolSnapshotPanel tools={execution.toolsSnapshot} />
        </TabsContent>

        <TabsContent value="metadata" className="pt-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <MetaCard
              icon={Bot}
              title="运行身份"
              items={[
                ["Agent", detail.session.agentName],
                ["Execution", execution.id],
                ["Session", execution.sessionId],
              ]}
            />
            <MetaCard
              icon={SquareTerminal}
              title="模型配置"
              items={[
                ["Provider", execution.provider],
                ["Model", execution.model],
                ["Thinking", execution.thinking],
              ]}
            />
            <MetaCard
              icon={Clock3}
              title="执行检查点"
              items={[
                ["Status", execution.status],
                ["Current tool", execution.currentTool || "—"],
                ["Checkpoint", execution.checkpoint || "尚未记录"],
                [
                  "Checkpoint time",
                  execution.checkpointAt
                    ? formatTime(execution.checkpointAt)
                    : "—",
                ],
              ]}
            />
            <MetaCard
              icon={Coins}
              title="资源用量"
              items={[
                ["Tokens", formatTokens(execution.tokens)],
                ["Input", formatTokens(execution.inputTokens)],
                ["Output", formatTokens(execution.outputTokens)],
                ["Cache read", formatTokens(execution.cacheReadTokens)],
                ["Cache write", formatTokens(execution.cacheWriteTokens)],
                ["Cost", formatCost(execution.cost)],
                ["Messages", String(detail.messagesPage.total)],
                ["Approvals", String(detail.approvals.length)],
              ]}
            />
            <MetaCard
              icon={Coins}
              title="价格快照（USD / 1M）"
              items={[
                ["Input", String(pricing.input)],
                ["Output", String(pricing.output)],
                ["Cache read", String(pricing.cacheRead)],
                ["Cache write", String(pricing.cacheWrite)],
              ]}
            />
            <MetaCard
              icon={Clock3}
              title="时间"
              items={[
                ["Started", formatTime(execution.startedAt)],
                ["Updated", formatTime(execution.updatedAt)],
                ["Finished", formatTime(execution.finishedAt)],
                [
                  "Duration",
                  formatDuration(execution.startedAt, execution.finishedAt),
                ],
              ]}
            />
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function WorkProgressPanel({
  updates,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  updates: ExecutionProgress[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
}) {
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const count = updates.length + (hasMore ? 1 : 0)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => viewportRef.current,
    estimateSize: (index) => (hasMore && index === 0 ? 52 : 220),
    getItemKey: (index) => {
      if (hasMore && index === 0) return "load-more-progress"
      return updates[index - (hasMore ? 1 : 0)]?.id ?? index
    },
    overscan: 5,
  })
  return (
    <Card className="h-[calc(100svh-17rem)] min-h-[520px] gap-0 py-0">
      <CardHeader className="shrink-0 border-b py-4">
        <CardTitle className="flex items-center gap-2">
          <ListChecks className="size-4" />
          工作进度
        </CardTitle>
        <CardDescription>
          Agent 完成阶段性工作后主动记录；最后一项表示它当前正在进行的工作。
        </CardDescription>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 p-0">
        {updates.length === 0 ? (
          <Empty className="h-full border-0">
            <EmptyHeader>
              <EmptyTitle>还没有阶段进度</EmptyTitle>
              <EmptyDescription>
                Agent 完成第一个实质阶段并调用进度工具后，这里会出现记录。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea viewportRef={viewportRef} className="h-full">
            <div
              className="relative mx-auto w-full max-w-4xl"
              style={{ height: virtualizer.getTotalSize() + 32 }}
            >
              {virtualizer.getVirtualItems().map((virtualItem) => {
                const offset = hasMore ? 1 : 0
                const index = virtualItem.index - offset
                const update = updates[index]
                const latest = index === updates.length - 1
                return (
                  <div
                    key={virtualItem.key}
                    ref={virtualizer.measureElement}
                    data-index={virtualItem.index}
                    className="absolute top-0 left-0 w-full px-4 pb-3 sm:px-6"
                    style={{
                      transform: `translateY(${virtualItem.start + 16}px)`,
                    }}
                  >
                    {!update ? (
                      <div className="flex justify-center py-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={loadingMore}
                          onClick={onLoadMore}
                        >
                          {loadingMore ? (
                            <Spinner data-icon="inline-start" />
                          ) : null}
                          加载更早进度
                        </Button>
                      </div>
                    ) : (
                      <Card size="sm">
                        <CardHeader>
                          <div className="flex items-start justify-between gap-4">
                            <div className="flex min-w-0 items-start gap-3">
                              <CircleCheckBig className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                              <div className="flex min-w-0 flex-col gap-1">
                                <CardTitle>{update.stage}</CardTitle>
                                <CardDescription>
                                  阶段 {index + 1} ·{" "}
                                  {formatTime(update.createdAt)}
                                </CardDescription>
                              </div>
                            </div>
                            {latest ? (
                              <Badge variant="secondary">当前</Badge>
                            ) : null}
                          </div>
                        </CardHeader>
                        <CardContent className="flex flex-col gap-4">
                          <div className="flex flex-col gap-1.5">
                            <p className="text-xs font-medium text-muted-foreground">
                              阶段结果
                            </p>
                            <MarkdownContent>{update.summary}</MarkdownContent>
                          </div>
                          <div className="flex items-start gap-2">
                            <Clock3 className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                            <div className="flex min-w-0 flex-col gap-1">
                              <p className="text-xs font-medium text-muted-foreground">
                                {latest ? "现在正在进行" : "随后开始"}
                              </p>
                              <p className="text-sm leading-relaxed whitespace-pre-wrap">
                                {update.currentActivity}
                              </p>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    )}
                  </div>
                )
              })}
            </div>
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  )
}

function ToolSnapshotPanel({ tools }: { tools: ToolSnapshot[] }) {
  return (
    <Card className="h-[calc(100svh-17rem)] min-h-[520px] gap-0 py-0">
      <CardHeader className="shrink-0 border-b py-4">
        <CardTitle className="flex items-center gap-2">
          <Wrench className="size-4" />
          工具快照
        </CardTitle>
        <CardDescription>
          Session 创建时实际启用的工具定义；后续修改 Agent 不会影响这里。
        </CardDescription>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 p-0">
        {tools.length === 0 ? (
          <Empty className="h-full border-0">
            <EmptyHeader>
              <EmptyTitle>该 Session 没有启用工具</EmptyTitle>
              <EmptyDescription>
                创建 Session 时使用了无工具运行模式。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea className="h-full">
            <div className="flex flex-col gap-3 p-4">
              {tools.map((tool) => (
                <ToolDefinitionItem key={tool.name} tool={tool} />
              ))}
            </div>
          </ScrollArea>
        )}
      </CardContent>
    </Card>
  )
}

function ToolDefinitionItem({ tool }: { tool: ToolSnapshot }) {
  const [open, setOpen] = React.useState(false)
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card size="sm">
        <CollapsibleTrigger
          className="w-full text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
          aria-label={`${open ? "收起" : "展开"}工具 ${tool.name}`}
        >
          <CardHeader>
            <div className="flex items-start justify-between gap-3">
              <div className="flex min-w-0 flex-col gap-1">
                <CardTitle className="font-mono">{tool.name}</CardTitle>
                <div className="flex flex-wrap gap-1.5">
                  <Badge variant="secondary">
                    {toolSourceLabel(tool.source)}
                  </Badge>
                  <Badge variant="outline">
                    {tool.parameters.length} 个参数
                  </Badge>
                </div>
              </div>
              {open ? (
                <ChevronUp className="size-4 shrink-0 text-muted-foreground" />
              ) : (
                <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
              )}
            </div>
          </CardHeader>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-1">
              <p className="text-xs font-medium text-muted-foreground">
                工具描述
              </p>
              <p className="text-sm leading-relaxed">{tool.description}</p>
            </div>
            <div className="flex flex-col gap-2">
              <p className="text-xs font-medium text-muted-foreground">
                参数 Schema
              </p>
              {tool.parameters.length > 0 ? (
                <ToolParameterList parameters={tool.parameters} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  没有可用的参数元数据。
                </p>
              )}
            </div>
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}

function ToolParameterList({
  parameters,
}: {
  parameters: ToolParameterSnapshot[]
}) {
  return (
    <div className="flex flex-col gap-2">
      {parameters.map((parameter) => (
        <div
          key={parameter.name}
          className="flex flex-col gap-2 rounded-lg bg-muted/40 p-3"
        >
          <div className="flex flex-wrap items-center gap-1.5">
            <code className="font-mono text-xs">{parameter.name}</code>
            <Badge variant="outline">{parameter.type}</Badge>
            <Badge variant={parameter.required ? "secondary" : "ghost"}>
              {parameter.required ? "必填" : "可选"}
            </Badge>
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">
            {parameter.description || "没有参数说明。"}
          </p>
          {parameter.enum && parameter.enum.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-muted-foreground">可选值</span>
              {parameter.enum.map((value) => (
                <Badge key={value} variant="outline">
                  {value}
                </Badge>
              ))}
            </div>
          )}
          {parameter.children && parameter.children.length > 0 && (
            <div className="ml-2 border-l pl-3">
              <ToolParameterList parameters={parameter.children} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

function toolSourceLabel(source: ToolSnapshot["source"]) {
  if (source === "pi_builtin") return "Pi 内置"
  if (source === "aegis_extension") return "Aegis 扩展"
  return "未知来源"
}

function PromptCard({
  icon: Icon,
  title,
  description,
  content,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  description: string
  content: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon className="size-4" />
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {content ? (
          <MarkdownContent className="text-sm">{content}</MarkdownContent>
        ) : (
          <Empty className="min-h-64 border-0">
            <EmptyHeader>
              <EmptyTitle>提示词尚未生成</EmptyTitle>
              <EmptyDescription>
                Session 启动后会在这里保存完整的提示词快照。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </CardContent>
    </Card>
  )
}

function MetaCard({
  icon: Icon,
  title,
  items,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  items: Array<[string, string]>
}) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="flex flex-col gap-3">
          {items.map(([label, value]) => (
            <div key={label} className="flex flex-col gap-1">
              <dt className="text-xs text-muted-foreground">{label}</dt>
              <dd className="font-mono text-xs break-all">{value || "—"}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}
