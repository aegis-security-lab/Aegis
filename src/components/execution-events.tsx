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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
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
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { formatTime } from "@/lib/format"
import type { ExecutionEvent, Issue } from "@/types"

type JsonRecord = Record<string, unknown>

interface TimelineItem {
  event: ExecutionEvent
  result?: ExecutionEvent
}

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
  const items = React.useMemo(() => groupLegacyToolEvents(events), [events])
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
      {items.map((item) => (
        <EventCard key={item.event.id} item={item} issues={issues} />
      ))}
    </div>
  )
}

function EventCard({ item, issues }: { item: TimelineItem; issues: Issue[] }) {
  const [open, setOpen] = React.useState(false)
  const { event } = item
  const tool = toolData(item)
  const summary = tool ? summarizeTool(tool) : null
  const icon = summary?.icon ?? eventIcon(event)
  const status = tool?.status || event.status
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="w-full text-left">
          <CardHeader className="flex flex-row items-center gap-3 px-4 py-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <EventIcon name={icon} />
            </span>
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <CardTitle className="truncate text-sm font-medium">
                {summary?.title ?? event.title}
              </CardTitle>
              {summary?.meta ? (
                <span className="shrink-0 text-xs text-muted-foreground">
                  {summary.meta}
                </span>
              ) : null}
            </div>
            {status ? <StatusBadge status={status} /> : null}
            <time className="hidden shrink-0 text-xs text-muted-foreground sm:block">
              {formatTime(event.createdAt)}
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
              <ToolDetails tool={tool} issues={issues} />
            ) : (
              <EventDetails event={event} />
            )}
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
  legacyInput: string
  legacyOutput: string
  isError: boolean
}

function toolData(item: TimelineItem): ToolData | null {
  if (item.event.type !== "tool" && item.event.type !== "tool_result")
    return null
  const event =
    item.event.type === "tool" ? item.event : (item.result ?? item.event)
  const result = item.result
  const name =
    event.toolName ||
    toolNameFromTitle(event.title) ||
    toolNameFromTitle(result?.title ?? "") ||
    "tool"
  const input = asRecord(parseJSON(event.inputJson))
  const output = asRecord(parseJSON(event.outputJson || result?.outputJson))
  const legacyOutput =
    result?.detail ||
    (item.event.type === "tool_result" ? item.event.detail : "")
  const legacyIsError =
    /Validation failed for tool|Tool execution failed|Error executing tool/i.test(
      legacyOutput
    )
  return {
    name,
    status:
      event.status ||
      (result
        ? result.isError || legacyIsError
          ? "failed"
          : "completed"
        : "completed"),
    input,
    output,
    legacyInput: event.detail || "",
    legacyOutput,
    isError: Boolean(event.isError || result?.isError || legacyIsError),
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
      return {
        title: `写入 · ${path || "文件"}`,
        meta: `${lineCount(content)} 行`,
        icon: "edit" as const,
      }
    }
    case "edit": {
      const edits = asArray(tool.input.edits)
      const lines = edits.reduce<number>(
        (total, edit) => total + lineCount(stringValue(asRecord(edit).newText)),
        0
      )
      return {
        title: `编辑 · ${path || "文件"}`,
        meta: `${edits.length || 1} 处 · ${lines} 行`,
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
        asArray(tool.input.children).length || createdChildren(tool).length
      return {
        title: "创建子 Issues",
        meta: `${count} 个`,
        icon: "subissues" as const,
      }
    }
    case "grep":
      return {
        title: `搜索 · ${stringValue(tool.input.pattern) || legacyField(tool.legacyInput, "pattern")}`,
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
          <CodeSection label="执行命令" value={toolCommand(tool)} />
          <CodeSection
            label={tool.isError ? "错误输出" : "命令输出"}
            value={output}
            empty="命令没有输出"
          />
        </div>
      )
    case "aegis_create_subissues":
      return <SubIssueDetails tool={tool} issues={issues} />
    default:
      return (
        <div className="flex flex-col gap-4">
          <JSONSection
            label="参数"
            value={tool.input}
            fallback={tool.legacyInput}
          />
          <JSONSection
            label="结果"
            value={tool.output}
            fallback={tool.legacyOutput}
          />
        </div>
      )
  }
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
                className="text-sm font-medium hover:underline"
              >
                {title || "未命名 Issue"}
              </Link>
            ) : (
              <p className="text-sm font-medium">{title || "未命名 Issue"}</p>
            )}
            {stringValue(spec.acceptanceCriteria) ? (
              <MarkdownContent className="text-xs leading-5 text-muted-foreground">
                {stringValue(spec.acceptanceCriteria)}
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
}: {
  label: string
  value: string
  empty?: string
}) {
  return (
    <section className="flex min-w-0 flex-col gap-2">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      {value ? (
        <ScrollArea className="max-h-80 rounded-lg border bg-muted/30">
          <pre className="min-w-max p-4 font-mono text-xs leading-5 whitespace-pre-wrap">
            {value}
          </pre>
        </ScrollArea>
      ) : (
        <p className="text-sm text-muted-foreground">{empty}</p>
      )}
    </section>
  )
}

function JSONSection({
  label,
  value,
  fallback,
}: {
  label: string
  value: JsonRecord
  fallback: string
}) {
  const content =
    Object.keys(value).length > 0 ? JSON.stringify(value, null, 2) : fallback
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

function groupLegacyToolEvents(events: ExecutionEvent[]): TimelineItem[] {
  const ordered = [...events].sort((a, b) =>
    a.createdAt.localeCompare(b.createdAt)
  )
  const items: TimelineItem[] = []
  const pending = new Map<string, TimelineItem[]>()
  for (const event of ordered) {
    if (event.type === "tool") {
      const item: TimelineItem = { event }
      items.push(item)
      if (!event.toolCallId && !event.outputJson) {
        const name = event.toolName || toolNameFromTitle(event.title) || "tool"
        const queue = pending.get(name) ?? []
        queue.push(item)
        pending.set(name, queue)
      }
      continue
    }
    if (event.type === "tool_result") {
      const name = event.toolName || toolNameFromTitle(event.title) || "tool"
      const queue = pending.get(name) ?? []
      const matchedIndex =
        name === "read"
          ? queue.findIndex((item) => legacyReadResultMatches(item, event))
          : -1
      const target = queue.splice(matchedIndex >= 0 ? matchedIndex : 0, 1)[0]
      if (target) {
        target.result = event
      } else {
        items.push({ event })
      }
      continue
    }
    items.push({ event })
  }
  return items.sort((a, b) =>
    b.event.createdAt.localeCompare(a.event.createdAt)
  )
}

function legacyReadResultMatches(item: TimelineItem, result: ExecutionEvent) {
  const tool = toolData(item)
  const path = tool ? toolPath(tool).toLowerCase() : ""
  const output = legacyTextOutput(result.detail).trimStart()
  if (path.endsWith(".json"))
    return output.startsWith("{") || output.startsWith("[")
  if (path.endsWith("go.mod")) return /^module\s+/i.test(output)
  if (path.endsWith(".md")) return /^#{1,6}\s/.test(output)
  return false
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

function toolNameFromTitle(title: string) {
  const start = title.match(/^调用\s+(.+)$/)
  if (start) return start[1]
  const end = title.match(/^(.+?)\s+执行完成$/)
  return end?.[1] ?? ""
}

function toolPath(tool: ToolData) {
  return (
    stringValue(
      tool.input.path || tool.input.filePath || tool.input.file_path
    ) ||
    legacyField(tool.legacyInput, "path") ||
    legacyField(tool.legacyInput, "filePath")
  )
}

function toolCommand(tool: ToolData) {
  return (
    stringValue(tool.input.command) || legacyField(tool.legacyInput, "command")
  )
}

function toolOutput(tool: ToolData) {
  const content = asArray(tool.output.content)
    .map((part) => stringValue(asRecord(part).text))
    .filter(Boolean)
    .join("\n")
  return content || legacyTextOutput(tool.legacyOutput)
}

function createdChildren(tool: ToolData): JsonRecord[] {
  const structured = asArray(asRecord(tool.output.details).children).map(
    asRecord
  )
  if (structured.length > 0) return structured
  const text = legacyTextOutput(tool.legacyOutput)
  const created = text.match(
    /(?:Created|Reused)\s+\d+\s+child Issues:\s*([\s\S]*?)\.\s*End this turn/i
  )?.[1]
  if (!created) return []
  return created.split(/,\s*/).map<JsonRecord>((entry) => {
    const match = entry.trim().match(/^(\S+)\s+([\s\S]+)$/)
    return match ? { identifier: match[1], title: match[2] } : { title: entry }
  })
}

function legacyField(detail: string, key: string) {
  if (!detail) return ""
  if (key === "command") {
    return (
      detail
        .match(/command:([\s\S]*?)(?:\s+timeout:[^\]]+)?\]$/)?.[1]
        ?.trim() ?? ""
    )
  }
  const fieldPattern = new RegExp(
    `${key}:([\\s\\S]*?)(?=\\s+(?:path|filePath|file_path|offset|limit|pattern|glob|timeout):|\\]$)`
  )
  return detail.match(fieldPattern)?.[1]?.trim() ?? ""
}

function legacyTextOutput(detail: string) {
  const start = detail.indexOf("text:")
  if (start < 0) return detail
  const content = detail.slice(start + "text:".length)
  const end = content.lastIndexOf(" type:text")
  return end >= 0 ? content.slice(0, end) : content
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
