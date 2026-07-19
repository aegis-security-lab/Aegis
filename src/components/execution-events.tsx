import * as React from "react"
import {
  CheckCircle2,
  ChevronRight,
  FilePenLine,
  FileText,
  FolderSearch,
  GitBranchPlus,
  Info,
  Search,
  TerminalSquare,
  Wrench,
  XCircle,
} from "lucide-react"
import { Link } from "react-router-dom"

import { MarkdownContent } from "@/components/markdown-content"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
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
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { formatTime } from "@/lib/format"
import { fetchExecutionEvent } from "@/lib/api"
import type { ExecutionEvent, Issue } from "@/types"

type JsonRecord = Record<string, unknown>

type EventIconName =
  | "completed"
  | "edit"
  | "error"
  | "file"
  | "folder"
  | "info"
  | "search"
  | "subissues"
  | "terminal"
  | "tool"

export function ExecutionEvents({
  events,
  issues = [],
}: {
  events: ExecutionEvent[]
  issues?: Issue[]
}) {
  const items = React.useMemo(
    () => [...events].sort((a, b) => b.createdAt.localeCompare(a.createdAt)),
    [events]
  )
  if (items.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>还没有事件</EmptyTitle>
          <EmptyDescription>
            Agent 开始执行后，工具调用会显示在这里。
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <div className="flex flex-col gap-2">
      {items.map((event) => (
        <EventCard key={event.id} event={event} issues={issues} />
      ))}
    </div>
  )
}

function EventCard({
  event,
  issues,
}: {
  event: ExecutionEvent
  issues: Issue[]
}) {
  const [open, setOpen] = React.useState(false)
  const [fullEvent, setFullEvent] = React.useState<ExecutionEvent | null>(null)
  const [detailError, setDetailError] = React.useState("")
  const [loadingDetail, setLoadingDetail] = React.useState(false)
  const resolvedEvent = fullEvent
    ? { ...fullEvent, status: event.status, updatedAt: event.updatedAt }
    : event
  const tool = toolData(resolvedEvent)
  const summary = tool ? summarizeTool(tool) : null
  const purpose = tool ? toolPurpose(tool) : ""
  const icon = summary?.icon ?? eventIcon(resolvedEvent)
  const status = tool?.status || resolvedEvent.status
  const toggle = (next: boolean) => {
    setOpen(next)
    if (!next || fullEvent || loadingDetail) return
    setDetailError("")
    setLoadingDetail(true)
    void fetchExecutionEvent(event.id)
      .then(setFullEvent)
      .catch((reason) =>
        setDetailError(
          reason instanceof Error ? reason.message : "读取事件详情失败"
        )
      )
      .finally(() => setLoadingDetail(false))
  }
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <Collapsible open={open} onOpenChange={toggle}>
        <CollapsibleTrigger className="w-full text-left">
          <CardHeader className="flex flex-row items-center gap-3 px-4 py-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <EventIcon name={icon} />
            </span>
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <CardTitle className="truncate text-sm font-medium">
                {purpose || summary?.title || event.title}
              </CardTitle>
              {summary?.meta ? (
                <span className="shrink-0 text-xs text-muted-foreground">
                  {summary.meta}
                </span>
              ) : null}
            </div>
            {status ? <StatusBadge status={status} /> : null}
            <time className="hidden shrink-0 text-xs text-muted-foreground sm:block">
              {formatTime(resolvedEvent.createdAt)}
            </time>
            <ChevronRight
              className={cn(
                "size-4 shrink-0 text-muted-foreground transition-transform",
                open && "rotate-90"
              )}
            />
          </CardHeader>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="border-t px-4 py-4">
            {tool ? (
              <div className="flex flex-col gap-5">
                <ToolDetails tool={tool} issues={issues} />
                {fullEvent ? (
                  <JSONSection label="原始参数" value={tool.input} />
                ) : loadingDetail ? (
                  <p className="text-xs text-muted-foreground">
                    正在读取原始参数…
                  </p>
                ) : null}
              </div>
            ) : (
              <EventDetails event={resolvedEvent} />
            )}
            {detailError ? (
              <p className="mt-3 text-xs text-destructive">{detailError}</p>
            ) : null}
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}

interface ToolData {
  name: string
  status: string
  input: JsonRecord
  output: JsonRecord
  isError: boolean
}

function toolData(event: ExecutionEvent): ToolData | null {
  if (event.type !== "tool") return null
  const input = asRecord(parseJSON(event.inputJson))
  const output = asRecord(parseJSON(event.outputJson))
  return {
    name: event.toolName || "tool",
    status: event.status || "running",
    input,
    output,
    isError: event.isError === true,
  }
}

function summarizeTool(tool: ToolData) {
  const path = toolPath(tool)
  switch (tool.name) {
    case "read":
      return {
        title: `读取 · ${path || "文件"}`,
        meta: "",
        icon: "file" as const,
      }
    case "write": {
      const content = stringValue(tool.input.content)
      const lines = Number(tool.input._lineCount) || lineCount(content)
      return {
        title: `写入 · ${path || "文件"}`,
        meta: `${lines} 行`,
        icon: "edit" as const,
      }
    }
    case "edit": {
      const edits = asArray(tool.input.edits)
      const lines =
        Number(tool.input._lineCount) ||
        edits.reduce<number>(
          (total, edit) =>
            total + lineCount(stringValue(asRecord(edit).newText)),
          0
        )
      const editCount = Number(tool.input._editCount) || edits.length || 1
      return {
        title: `编辑 · ${path || "文件"}`,
        meta: `${editCount} 处 · ${lines} 行`,
        icon: "edit" as const,
      }
    }
    case "bash":
      return {
        title: `Bash · ${oneLine(toolCommand(tool)) || "执行命令"}`,
        meta: "",
        icon: "terminal" as const,
      }
    case "aegis_create_subissues": {
      const count =
        Number(tool.input._childCount) ||
        asArray(tool.input.children).length ||
        createdChildren(tool).length
      return {
        title: "创建子 Issues",
        meta: `${count} 个`,
        icon: "subissues" as const,
      }
    }
    case "aegis_report_progress":
      return {
        title: `进度 · ${stringValue(tool.input.stage) || "阶段更新"}`,
        meta: oneLine(stringValue(tool.input.currentActivity)),
        icon: "info" as const,
      }
    case "aegis_broadcast": {
      const delivered = Number(asRecord(tool.output.details).deliveredCount)
      return {
        title: `广播 · ${stringValue(tool.input.subject) || "任务信息"}`,
        meta: Number.isFinite(delivered) ? `投递 ${delivered}` : "",
        icon: "info" as const,
      }
    }
    case "aegis_list_broadcasts": {
      const count = asArray(asRecord(tool.output.details).broadcasts).length
      return {
        title: "读取广播历史",
        meta: count ? `${count} 条` : "",
        icon: "info" as const,
      }
    }
    case "grep":
      return {
        title: `搜索 · ${stringValue(tool.input.pattern) || "内容"}`,
        meta: path,
        icon: "search" as const,
      }
    case "find":
    case "ls":
      return {
        title: `${tool.name} · ${path || "工作区"}`,
        meta: "",
        icon: "folder" as const,
      }
    default:
      return {
        title: `工具 · ${tool.name}`,
        meta: path,
        icon: "tool" as const,
      }
  }
}

function ToolDetails({ tool, issues }: { tool: ToolData; issues: Issue[] }) {
  const output = toolOutput(tool)
  switch (tool.name) {
    case "read":
      return (
        <CodeSection label="文件内容" value={output} empty="没有文本输出" />
      )
    case "write":
      return (
        <div className="flex flex-col gap-4">
          <PathLine action="写入" path={toolPath(tool)} />
          <CodeSection
            label="写入内容"
            value={stringValue(tool.input.content)}
          />
          {tool.isError ? <CodeSection label="错误" value={output} /> : null}
        </div>
      )
    case "edit":
      return <EditDetails tool={tool} />
    case "bash":
      return (
        <div className="flex flex-col gap-4">
          <CodeSection
            label="执行命令"
            value={toolCommand(tool)}
            maxVisibleLines={12}
          />
          <CodeSection
            label={tool.isError ? "错误输出" : "命令输出"}
            value={output}
            empty="命令没有输出"
            maxVisibleLines={16}
          />
        </div>
      )
    case "aegis_create_subissues":
      return <SubIssueDetails tool={tool} issues={issues} />
    case "aegis_report_progress":
      return (
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <p className="text-xs font-medium text-muted-foreground">
              阶段结果
            </p>
            <MarkdownContent>
              {stringValue(tool.input.summary) || "没有阶段总结"}
            </MarkdownContent>
          </div>
          <div className="flex flex-col gap-1">
            <p className="text-xs font-medium text-muted-foreground">
              现在正在进行
            </p>
            <p className="text-sm leading-relaxed whitespace-pre-wrap">
              {stringValue(tool.input.currentActivity) || "未说明"}
            </p>
          </div>
        </div>
      )
    case "aegis_broadcast":
      return <BroadcastDetails tool={tool} />
    case "aegis_list_broadcasts":
      return <BroadcastHistoryDetails tool={tool} />
    default:
      return <JSONSection label="结果" value={tool.output} />
  }
}

function BroadcastDetails({ tool }: { tool: ToolData }) {
  const details = asRecord(tool.output.details)
  const delivered = Number(details.deliveredCount)
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-2">
        <Badge variant="secondary">
          {stringValue(tool.input.importance) || "normal"}
        </Badge>
        {Number.isFinite(delivered) ? (
          <Badge variant="outline">已投递 {delivered}</Badge>
        ) : null}
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-xs font-medium text-muted-foreground">广播内容</p>
        <MarkdownContent>
          {stringValue(tool.input.message) || "没有广播内容"}
        </MarkdownContent>
      </div>
    </div>
  )
}

function BroadcastHistoryDetails({ tool }: { tool: ToolData }) {
  const items = asArray(asRecord(tool.output.details).broadcasts).map(asRecord)
  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">没有历史广播。</p>
  }
  return (
    <div className="flex flex-col gap-3">
      {items.map((item, index) => (
        <Card key={stringValue(item.id) || index} size="sm">
          <CardHeader>
            <div className="flex items-start justify-between gap-3">
              <CardTitle>{stringValue(item.subject) || "任务广播"}</CardTitle>
              <Badge variant="outline">
                {stringValue(item.importance) || "normal"}
              </Badge>
            </div>
            <CardDescription>
              {stringValue(item.sourceAgentId)} · {stringValue(item.createdAt)}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <MarkdownContent>{stringValue(item.message)}</MarkdownContent>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function EditDetails({ tool }: { tool: ToolData }) {
  const edits = asArray(tool.input.edits)
  const details = asRecord(tool.output.details)
  const diff = stringValue(details.diff || details.patch)
  return (
    <div className="flex flex-col gap-4">
      <PathLine action="编辑" path={toolPath(tool)} />
      {diff ? <CodeSection label="变更 Diff" value={diff} /> : null}
      {edits.map((edit, index) => {
        const item = asRecord(edit)
        return (
          <div key={index} className="grid gap-3 lg:grid-cols-2">
            <CodeSection
              label={`替换前 ${index + 1}`}
              value={stringValue(item.oldText)}
            />
            <CodeSection
              label={`替换后 ${index + 1}`}
              value={stringValue(item.newText)}
            />
          </div>
        )
      })}
      {tool.isError ? (
        <CodeSection label="错误" value={toolOutput(tool)} />
      ) : null}
    </div>
  )
}

function SubIssueDetails({
  tool,
  issues,
}: {
  tool: ToolData
  issues: Issue[]
}) {
  const requested = asArray(tool.input.children).map(asRecord)
  const created = createdChildren(tool)
  const items = requested.length > 0 ? requested : created
  return (
    <div className="flex flex-col gap-3">
      {items.map((spec, index) => {
        const actual = created[index] ?? spec
        const issue = issues.find(
          (candidate) =>
            candidate.id === stringValue(actual.id) ||
            candidate.title === stringValue(spec.title)
        )
        const id = stringValue(actual.id || issue?.id)
        const identifier = stringValue(actual.identifier || issue?.identifier)
        const title = stringValue(spec.title || actual.title || issue?.title)
        const dependsOn = asArray(spec.dependsOn).map(String)
        return (
          <article
            key={id || `${title}-${index}`}
            className="flex flex-col gap-2 rounded-lg border p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              {identifier ? (
                <Badge variant="outline">{identifier}</Badge>
              ) : null}
              {stringValue(spec.priority) ? (
                <Badge variant="secondary">{stringValue(spec.priority)}</Badge>
              ) : null}
              {stringValue(spec.agentId) ? (
                <Badge variant="secondary">{stringValue(spec.agentId)}</Badge>
              ) : null}
              {dependsOn.length > 0 ? (
                <Badge variant="outline">依赖 #{dependsOn.join(", #")}</Badge>
              ) : null}
            </div>
            {id ? (
              <Link
                to={`/issues/${id}`}
                className="line-clamp-2 text-sm font-medium break-all hover:underline"
                title={title}
              >
                {title || "未命名 Issue"}
              </Link>
            ) : (
              <p
                className="line-clamp-2 text-sm font-medium break-all"
                title={title}
              >
                {title || "未命名 Issue"}
              </p>
            )}
            {stringValue(spec.objective) ? (
              <MarkdownContent className="text-xs leading-5 text-muted-foreground">
                {stringValue(spec.objective)}
              </MarkdownContent>
            ) : null}
          </article>
        )
      })}
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          没有可解析的子 Issue 数据。
        </p>
      ) : null}
    </div>
  )
}

function EventDetails({ event }: { event: ExecutionEvent }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">{event.type}</Badge>
        <span className="text-xs text-muted-foreground">
          {formatTime(event.createdAt)}
        </span>
      </div>
      {event.detail ? (
        <section className="flex min-w-0 flex-col gap-2">
          <p className="text-xs font-medium text-muted-foreground">详情</p>
          <MarkdownContent className="text-sm">{event.detail}</MarkdownContent>
        </section>
      ) : (
        <p className="text-sm text-muted-foreground">没有更多详情。</p>
      )}
    </div>
  )
}

function CodeSection({
  label,
  value,
  empty = "没有内容",
  maxVisibleLines = 16,
}: {
  label: string
  value: string
  empty?: string
  maxVisibleLines?: number
}) {
  const visibleLines = Math.min(Math.max(lineCount(value), 1), maxVisibleLines)
  const viewportHeight = visibleLines * 20 + 34
  return (
    <section className="flex min-w-0 flex-col gap-2">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      {value ? (
        <ScrollArea
          className="w-full overflow-hidden rounded-lg border bg-muted/30"
          style={{ height: viewportHeight }}
        >
          <pre className="w-max min-w-full p-4 font-mono text-xs leading-5 whitespace-pre">
            {value}
          </pre>
          <ScrollBar orientation="horizontal" />
        </ScrollArea>
      ) : (
        <p className="text-sm text-muted-foreground">{empty}</p>
      )}
    </section>
  )
}

function JSONSection({ label, value }: { label: string; value: JsonRecord }) {
  const content =
    Object.keys(value).length > 0 ? JSON.stringify(value, null, 2) : ""
  return <CodeSection label={label} value={content} />
}

function PathLine({ action, path }: { action: string; path: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <Badge variant="secondary">{action}</Badge>
      <code className="min-w-0 truncate text-xs">{path || "未知路径"}</code>
    </div>
  )
}

function eventIcon(event: ExecutionEvent): EventIconName {
  if (event.type === "error" || event.type === "stderr") return "error"
  if (event.type === "worker" || event.type === "continuation")
    return "completed"
  return "info"
}

function EventIcon({ name }: { name: EventIconName }) {
  const className = "size-4"
  switch (name) {
    case "completed":
      return <CheckCircle2 className={className} />
    case "edit":
      return <FilePenLine className={className} />
    case "error":
      return <XCircle className={className} />
    case "file":
      return <FileText className={className} />
    case "folder":
      return <FolderSearch className={className} />
    case "search":
      return <Search className={className} />
    case "subissues":
      return <GitBranchPlus className={className} />
    case "terminal":
      return <TerminalSquare className={className} />
    case "tool":
      return <Wrench className={className} />
    default:
      return <Info className={className} />
  }
}

function toolPath(tool: ToolData) {
  return stringValue(tool.input.path)
}

function toolPurpose(tool: ToolData) {
  return oneLine(stringValue(tool.input.description))
}

function toolCommand(tool: ToolData) {
  return stringValue(tool.input.command)
}

function toolOutput(tool: ToolData) {
  const content = asArray(tool.output.content)
    .map((part) => stringValue(asRecord(part).text))
    .filter(Boolean)
    .join("\n")
  return content || stringValue(tool.output.text)
}

function createdChildren(tool: ToolData): JsonRecord[] {
  const structured = asArray(asRecord(tool.output.details).children).map(
    asRecord
  )
  return structured
}

function parseJSON(value?: string): unknown {
  if (!value) return null
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function asRecord(value: unknown): JsonRecord {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as JsonRecord)
    : {}
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function stringValue(value: unknown) {
  return typeof value === "string" || typeof value === "number"
    ? String(value)
    : ""
}

function lineCount(value: string) {
  const normalized = value.replace(/\r\n/g, "\n").replace(/\n$/, "")
  return normalized ? normalized.split("\n").length : 0
}

function oneLine(value: string) {
  const result = value.replace(/\s+/g, " ").trim()
  return result.length > 120 ? `${result.slice(0, 117)}…` : result
}
