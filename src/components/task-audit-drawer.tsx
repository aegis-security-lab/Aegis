import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronDown,
  Download,
  FileSearch,
  Plus,
  Search,
  Wrench,
} from "lucide-react"
import { toast } from "sonner"

import { MarkdownContent } from "@/components/markdown-content"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import {
  createTaskAudit,
  fetchTaskAudit,
  fetchTaskAudits,
  taskAuditReportURL,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { TaskAudit, TaskAuditEvent } from "@/types"

interface TaskAuditDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  taskId: string
  taskTitle: string
}

export function TaskAuditDrawer({
  open,
  onOpenChange,
  taskId,
  taskTitle,
}: TaskAuditDrawerProps) {
  const [audits, setAudits] = React.useState<TaskAudit[]>([])
  const [selectedId, setSelectedId] = React.useState("")
  const [loading, setLoading] = React.useState(false)
  const [creating, setCreating] = React.useState(false)
  const [traceOpen, setTraceOpen] = React.useState(true)
  const traceEndRef = React.useRef<HTMLDivElement>(null)

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const result = await fetchTaskAudits(taskId)
      setAudits(result.audits)
      setSelectedId((current) =>
        current && result.audits.some((audit) => audit.id === current)
          ? current
          : (result.audits[0]?.id ?? "")
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取审计记录失败")
    } finally {
      setLoading(false)
    }
  }, [taskId])

  React.useEffect(() => {
    if (open) void load()
  }, [open, load])

  const selected = audits.find((audit) => audit.id === selectedId)
  const selectedStatus = selected?.status

  React.useEffect(() => {
    if (!open || !selectedId) return
    void fetchTaskAudit(selectedId)
      .then((next) =>
        setAudits((current) =>
          current.map((audit) => (audit.id === next.id ? next : audit))
        )
      )
      .catch((error) =>
        toast.error(error instanceof Error ? error.message : "读取审计详情失败")
      )
  }, [open, selectedId])

  React.useEffect(() => {
    if (!selectedStatus) return
    setTraceOpen(["preparing", "running"].includes(selectedStatus))
  }, [selectedId, selectedStatus])

  React.useEffect(() => {
    if (!traceOpen || selected?.status !== "running") return
    traceEndRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "nearest",
    })
  }, [
    traceOpen,
    selected?.status,
    selected?.events?.length,
    selected?.updatedAt,
  ])

  React.useEffect(() => {
    if (
      !open ||
      !selected ||
      !["preparing", "running"].includes(selected.status)
    )
      return
    const timer = window.setInterval(() => {
      void fetchTaskAudit(selected.id)
        .then((next) =>
          setAudits((current) =>
            current.map((audit) => (audit.id === next.id ? next : audit))
          )
        )
        .catch(() => undefined)
    }, 700)
    return () => window.clearInterval(timer)
  }, [open, selected])

  const create = async () => {
    setCreating(true)
    try {
      const audit = await createTaskAudit(taskId)
      setAudits((current) => [audit, ...current])
      setSelectedId(audit.id)
      toast.success("任务审计已启动")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "启动任务审计失败")
    } finally {
      setCreating(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(96vw,960px)]! gap-0 p-0 sm:max-w-[960px]!">
        <SheetHeader className="border-b px-5 py-4 pr-14">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <SheetTitle className="flex items-center gap-2">
                <FileSearch className="size-4 text-primary" />
                任务审计
              </SheetTitle>
              <SheetDescription className="mt-1 line-clamp-1">
                {taskTitle} · 基于冻结证据快照评估任务质量与平台健康度
              </SheetDescription>
            </div>
            <Button size="sm" onClick={() => void create()} disabled={creating}>
              {creating ? <Spinner /> : <Plus data-icon="inline-start" />}
              新建审计
            </Button>
          </div>
        </SheetHeader>

        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[224px_minmax(0,1fr)]">
          <aside className="max-h-48 overflow-y-auto border-b bg-muted/20 p-3 md:max-h-none md:border-r md:border-b-0">
            <p className="px-2 pb-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
              审计记录 · {audits.length}
            </p>
            {loading && audits.length === 0 ? (
              <div className="flex h-20 items-center justify-center">
                <Spinner />
              </div>
            ) : audits.length === 0 ? (
              <p className="rounded-md border border-dashed p-3 text-xs leading-5 text-muted-foreground">
                暂无记录。新建审计后会冻结当前任务证据并实时生成报告。
              </p>
            ) : (
              <div className="space-y-1">
                {audits.map((audit, index) => (
                  <button
                    key={audit.id}
                    type="button"
                    onClick={() => setSelectedId(audit.id)}
                    className={cn(
                      "w-full rounded-md border px-3 py-2.5 text-left transition-colors",
                      selectedId === audit.id
                        ? "border-primary/35 bg-background"
                        : "border-transparent hover:bg-background/70"
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs font-medium">
                        第 {audits.length - index} 次
                      </span>
                      <AuditStatus audit={audit} compact />
                    </div>
                    <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                      {formatTime(audit.createdAt)}
                    </p>
                  </button>
                ))}
              </div>
            )}
          </aside>

          <main className="flex min-h-0 min-w-0 flex-col">
            {selected ? (
              <>
                <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-3">
                  <div className="flex items-center gap-2">
                    <AuditStatus audit={selected} />
                    <span className="text-xs text-muted-foreground">
                      {selected.frameworkVersion}
                    </span>
                    {!selected.evidenceComplete ? (
                      <Badge variant="outline" className="text-warning">
                        证据快照有警告
                      </Badge>
                    ) : null}
                  </div>
                  {selected.reportMarkdown ? (
                    <Button
                      variant="outline"
                      size="sm"
                      render={
                        <a href={taskAuditReportURL(selected.id)} download />
                      }
                      nativeButton={false}
                    >
                      <Download data-icon="inline-start" />
                      下载报告
                    </Button>
                  ) : null}
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto bg-background px-5 py-5 md:px-8 md:py-7">
                  <div className="mx-auto max-w-3xl space-y-6">
                    <AuditTrace
                      events={selected.events ?? []}
                      status={selected.status}
                      open={traceOpen}
                      onOpenChange={setTraceOpen}
                      endRef={traceEndRef}
                    />

                    {selected.reportMarkdown &&
                    selected.status === "completed" ? (
                      <section aria-labelledby="audit-report-title">
                        <div className="mb-5 flex items-center gap-3">
                          <div className="flex size-8 items-center justify-center rounded-md bg-primary/10 text-primary">
                            <FileSearch className="size-4" />
                          </div>
                          <div>
                            <h3
                              id="audit-report-title"
                              className="text-sm font-semibold"
                            >
                              最终审计报告
                            </h3>
                            <p className="text-xs text-muted-foreground">
                              已通过结构校验并完成归档
                            </p>
                          </div>
                        </div>
                        <article className="border-l-2 border-primary/20 pl-5 md:pl-7">
                          <MarkdownContent className="text-sm !leading-7">
                            {selected.reportMarkdown}
                          </MarkdownContent>
                        </article>
                      </section>
                    ) : selected.status === "failed" ? (
                      <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-5">
                        <p className="flex items-center gap-2 font-medium text-destructive">
                          <AlertTriangle className="size-4" />
                          审计失败
                        </p>
                        <p className="mt-2 text-sm leading-6 text-muted-foreground">
                          {selected.error || "分析 Agent 未能生成报告。"}
                        </p>
                      </div>
                    ) : (selected.events?.length ?? 0) === 0 ? (
                      <div className="flex min-h-64 flex-col items-center justify-center text-center">
                        <Spinner />
                        <p className="mt-4 text-sm font-medium">
                          {selected.status === "preparing"
                            ? "正在冻结任务证据"
                            : "分析 Agent 正在读取证据"}
                        </p>
                        <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">
                          AI
                          的输出、工具参数和执行结果会实时显示。关闭抽屉不会中断审计。
                        </p>
                      </div>
                    ) : null}
                  </div>
                </div>
              </>
            ) : (
              <div className="flex min-h-72 flex-1 flex-col items-center justify-center p-8 text-center">
                <FileSearch className="size-8 text-muted-foreground/60" />
                <p className="mt-3 text-sm font-medium">创建一次独立任务审计</p>
                <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">
                  分析 Agent 会读取任务、Issue
                  树、全部会话、工具事件、协作、验收、日志、Phone 与附件证据。
                </p>
              </div>
            )}
          </main>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function AuditTrace({
  events,
  status,
  open,
  onOpenChange,
  endRef,
}: {
  events: TaskAuditEvent[]
  status: TaskAudit["status"]
  open: boolean
  onOpenChange: (open: boolean) => void
  endRef: React.RefObject<HTMLDivElement | null>
}) {
  const toolCount = events.filter((event) => event.kind === "tool").length
  const latest = events.at(-1)
  const isActive = status === "preparing" || status === "running"

  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <div className="overflow-hidden rounded-lg border bg-muted/10">
        <CollapsibleTrigger className="group flex min-h-12 w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset">
          <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
            {isActive ? (
              <Spinner className="size-3.5" />
            ) : (
              <Bot className="size-3.5" />
            )}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold">审计轨迹</span>
              <span className="font-mono text-[10px] text-muted-foreground tabular-nums">
                {events.length} 条 · {toolCount} 次工具
              </span>
            </div>
            <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
              {isActive
                ? eventSummary(latest) || "等待分析 Agent 开始工作"
                : open
                  ? "收起执行历史"
                  : "已折叠，可展开复盘 AI 输出与工具调用"}
            </p>
          </div>
          <ChevronDown
            className={cn(
              "size-4 shrink-0 text-muted-foreground transition-transform duration-200",
              open && "rotate-180"
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="border-t px-4 py-2">
            {events.length === 0 ? (
              <div className="flex items-center gap-2 py-6 text-xs text-muted-foreground">
                {isActive ? <Spinner /> : <Bot className="size-3.5" />}
                {isActive
                  ? "等待第一条审计事件"
                  : "该历史记录创建时尚未采集执行轨迹"}
              </div>
            ) : (
              <ol className="relative space-y-0 before:absolute before:top-4 before:bottom-4 before:left-[13px] before:w-px before:bg-border">
                {events.map((event) => (
                  <AuditTraceEvent key={event.id} event={event} />
                ))}
              </ol>
            )}
            <div ref={endRef} />
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

function AuditTraceEvent({ event }: { event: TaskAuditEvent }) {
  if (event.kind === "tool") return <ToolTraceEvent event={event} />
  const failed = event.isError || event.status === "failed"
  return (
    <li className="relative grid grid-cols-[28px_minmax(0,1fr)] gap-3 py-2.5">
      <span
        className={cn(
          "relative z-10 flex size-7 items-center justify-center rounded-full border bg-background",
          failed
            ? "text-destructive"
            : event.kind === "assistant"
              ? "text-primary"
              : "text-muted-foreground"
        )}
      >
        {failed ? (
          <AlertTriangle className="size-3.5" />
        ) : event.kind === "assistant" ? (
          <Bot className="size-3.5" />
        ) : (
          <CheckCircle2 className="size-3.5" />
        )}
      </span>
      <div className="min-w-0 pt-1">
        <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
          <span className="font-medium text-foreground">
            {event.kind === "assistant" ? "AI 输出" : "系统"}
          </span>
          {event.status === "streaming" || event.status === "running" ? (
            <span className="flex items-center gap-1">
              <span className="size-1.5 animate-pulse rounded-full bg-primary" />
              进行中
            </span>
          ) : null}
          <time className="ml-auto font-mono text-[10px] tabular-nums">
            {formatTime(event.createdAt)}
          </time>
        </div>
        {event.content ? (
          event.kind === "assistant" ? (
            <MarkdownContent className="mt-1.5 text-xs !leading-5">
              {event.content}
            </MarkdownContent>
          ) : (
            <p
              className={cn(
                "mt-1 text-xs leading-5 text-muted-foreground",
                failed && "text-destructive"
              )}
            >
              {event.content}
            </p>
          )
        ) : (
          <p className="mt-1 text-xs text-muted-foreground">正在生成内容…</p>
        )}
      </div>
    </li>
  )
}

function ToolTraceEvent({ event }: { event: TaskAuditEvent }) {
  const [open, setOpen] = React.useState(
    event.status !== "completed" || event.isError
  )
  const active = event.status === "generating" || event.status === "running"
  return (
    <li className="relative grid grid-cols-[28px_minmax(0,1fr)] gap-3 py-2.5">
      <span
        className={cn(
          "relative z-10 flex size-7 items-center justify-center rounded-full border bg-background",
          event.isError ? "text-destructive" : "text-muted-foreground"
        )}
      >
        {event.toolName?.includes("search") ? (
          <Search className="size-3.5" />
        ) : (
          <Wrench className="size-3.5" />
        )}
      </span>
      <Collapsible open={open} onOpenChange={setOpen} className="min-w-0">
        <CollapsibleTrigger className="group flex min-h-7 w-full items-center gap-2 text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none">
          <code className="truncate text-[11px] font-medium text-foreground">
            {event.toolName || "正在生成工具调用"}
          </code>
          <span
            className={cn(
              "text-[10px]",
              event.isError ? "text-destructive" : "text-muted-foreground"
            )}
          >
            {event.isError ? "失败" : active ? "执行中" : "完成"}
          </span>
          {active ? <Spinner className="size-3" /> : null}
          <time className="ml-auto font-mono text-[10px] text-muted-foreground tabular-nums">
            {formatTime(event.createdAt)}
          </time>
          <ChevronDown
            className={cn(
              "size-3.5 text-muted-foreground transition-transform duration-200",
              open && "rotate-180"
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="mt-2 space-y-2 rounded-md border bg-background p-3">
            <TracePayload
              label={event.status === "generating" ? "参数生成中" : "调用参数"}
              value={event.arguments}
              active={event.status === "generating"}
            />
            {event.result ? (
              <TracePayload
                label={event.isError ? "错误" : "工具输出"}
                value={event.result}
                error={event.isError}
              />
            ) : null}
          </div>
        </CollapsibleContent>
      </Collapsible>
    </li>
  )
}

function TracePayload({
  label,
  value,
  active = false,
  error = false,
}: {
  label: string
  value?: string
  active?: boolean
  error?: boolean
}) {
  if (!value && !active) return null
  return (
    <div>
      <p
        className={cn(
          "mb-1.5 text-[10px] font-medium tracking-wide text-muted-foreground uppercase",
          error && "text-destructive"
        )}
      >
        {label}
      </p>
      <pre
        className={cn(
          "max-h-56 overflow-auto font-mono text-[10px] leading-4 break-all whitespace-pre-wrap text-muted-foreground",
          error && "text-destructive"
        )}
      >
        {formatTracePayload(value) || "等待参数…"}
        {active ? (
          <span className="ml-0.5 inline-block h-3 w-px animate-pulse bg-primary align-middle" />
        ) : null}
      </pre>
    </div>
  )
}

function formatTracePayload(value?: string) {
  if (!value) return ""
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function eventSummary(event?: TaskAuditEvent) {
  if (!event) return ""
  if (event.kind === "tool")
    return `${event.toolName || "工具参数"}${event.status === "completed" ? "已完成" : "正在执行"}`
  return event.content?.replace(/\s+/g, " ").slice(0, 100) || "AI 正在生成内容"
}

function AuditStatus({
  audit,
  compact = false,
}: {
  audit: TaskAudit
  compact?: boolean
}) {
  if (audit.status === "completed")
    return (
      <Badge variant="outline" className="text-success">
        <CheckCircle2 />
        {compact ? "完成" : "审计完成"}
      </Badge>
    )
  if (audit.status === "failed")
    return (
      <Badge variant="outline" className="text-destructive">
        <AlertTriangle />
        {compact ? "失败" : "审计失败"}
      </Badge>
    )
  return (
    <Badge variant="outline">
      <Spinner />
      {audit.status === "preparing" ? "准备证据" : "分析中"}
    </Badge>
  )
}
