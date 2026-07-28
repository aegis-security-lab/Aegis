import * as React from "react"
import {
  ChevronDown,
  ChevronRight,
  ClipboardList,
  Code2,
  Download,
  FileText,
  HeartPulse,
  LayoutTemplate,
  MessageSquareText,
  TerminalSquare,
} from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Attachment,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Message, MessageContent } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { fetchExecutionEvent } from "@/lib/api"
import type {
  Execution,
  ExecutionEvent,
  Issue,
  IssueAttachment,
  Message as ChatMessage,
} from "@/types"

interface EmployeeTurn {
  id: string
  user?: ChatMessage
  processMessages: ChatMessage[]
  events: ExecutionEvent[]
  final?: ChatMessage
  execution: Execution
  issue?: Issue
  attachments: IssueAttachment[]
  startedAt: string
  active: boolean
}

export function EmployeeActivityConversation({
  executions,
  messages,
  events,
  attachments,
  issues,
}: {
  executions: Execution[]
  messages: ChatMessage[]
  events: ExecutionEvent[]
  attachments: IssueAttachment[]
  issues: Issue[]
}) {
  const turns = React.useMemo(
    () => buildEmployeeTurns(executions, messages, events, attachments, issues),
    [attachments, events, executions, issues, messages]
  )
  const hasActiveExecution = executions.some(isExecutionActive)

  return (
    <MessageScrollerProvider
      defaultScrollPosition="end"
      autoScroll={hasActiveExecution}
    >
      <MessageScroller className="h-full min-h-0">
        <MessageScrollerViewport className="h-full px-5 py-4 sm:px-8">
          <MessageScrollerContent className="mx-auto w-full max-w-4xl gap-3">
            {turns.length === 0 ? (
              <div className="m-auto text-sm text-muted-foreground">
                这名员工还没有工作记录
              </div>
            ) : null}
            {turns.map((turn) => (
              <EmployeeTurnView key={turn.id} turn={turn} />
            ))}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

function EmployeeTurnView({ turn }: { turn: EmployeeTurn }) {
  return (
    <MessageScrollerItem messageId={turn.id} scrollAnchor>
      <div className="flex min-w-0 flex-col gap-2.5">
        {turn.user ? (
          promptKind(turn) ? (
            <PromptCard turn={turn} kind={promptKind(turn)!} />
          ) : (
            <Message align="end">
              <MessageContent>
                <Bubble variant="secondary" align="end">
                  <BubbleContent className="text-[13px] leading-5">
                    <MarkdownContent className="!leading-5 [&>*+*]:!mt-2">
                      {visibleUserContent(turn.user.content) || "（空消息）"}
                    </MarkdownContent>
                  </BubbleContent>
                </Bubble>
              </MessageContent>
            </Message>
          )
        ) : null}
        <ProcessArea
          key={`${turn.id}-${turn.active ? "active" : "settled"}`}
          execution={turn.execution}
          events={turn.events}
          messages={turn.processMessages}
          active={turn.active}
        />
        {turn.final?.content ? (
          <Message>
            <MessageContent>
              <MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">
                {turn.final.content}
              </MarkdownContent>
            </MessageContent>
          </Message>
        ) : null}
        <PublishedAttachments attachments={turn.attachments} issue={turn.issue} />
      </div>
    </MessageScrollerItem>
  )
}

function PublishedAttachments({
  attachments,
  issue,
}: {
  attachments: IssueAttachment[]
  issue?: Issue
}) {
  if (attachments.length === 0) return null
  return (
    <Message>
      <MessageContent className="w-full max-w-[46rem]">
        <AttachmentGroup>
          {attachments.map((attachment) => (
            <Attachment key={attachment.id} size="sm" className="w-80">
              <AttachmentMedia>
                <FileText />
              </AttachmentMedia>
              <AttachmentContent>
                <AttachmentTitle title={attachment.name}>
                  {attachment.name}
                </AttachmentTitle>
                <AttachmentDescription
                  title={attachment.description || attachment.sourcePath}
                >
                  {attachment.description || attachment.sourcePath}
                  {issue ? ` · ${issue.identifier}` : ""}
                  {` · ${formatBytes(attachment.size)}`}
                </AttachmentDescription>
              </AttachmentContent>
              <AttachmentActions>
                <a
                  className={buttonVariants({
                    variant: "ghost",
                    size: "icon-xs",
                  })}
                  aria-label={`下载 ${attachment.name}`}
                  title="下载附件"
                  href={`/api/attachments/${encodeURIComponent(attachment.id)}`}
                  download={attachment.name}
                >
                  <Download data-icon="inline-start" />
                </a>
              </AttachmentActions>
            </Attachment>
          ))}
        </AttachmentGroup>
      </MessageContent>
    </Message>
  )
}

function PromptCard({
  turn,
  kind,
}: {
  turn: EmployeeTurn
  kind: "task" | "comment" | "heartbeat" | "system"
}) {
  const [raw, setRaw] = React.useState(false)
  const issue = turn.issue
  const content = turn.user?.content ?? ""
  const comment = extractComment(content)
  const title =
    kind === "task"
      ? "接受 Board 委派"
      : kind === "comment"
        ? "Issue 评论"
        : kind === "heartbeat"
          ? "系统心跳"
          : "系统通知"
  const subtitle = issue
    ? `${issue.identifier} · ${issue.title}`
    : `${turn.execution.kind} · ${turn.execution.agentId}`
  return (
    <Message align="end">
      <MessageContent className="w-full max-w-[46rem]">
        <div className="group/prompt relative w-full rounded-xl border bg-card px-4 py-3">
          <div className="flex items-start gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              {kind === "task" ? (
                <ClipboardList className="size-4" />
              ) : kind === "comment" ? (
                <MessageSquareText className="size-4" />
              ) : (
                <HeartPulse className="size-4" />
              )}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold">{title}</p>
              <p className="mt-0.5 break-words text-xs leading-5 text-muted-foreground">
                {subtitle}
              </p>
            </div>
          </div>
          {raw ? (
            <pre className="mt-3 max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/60 p-3 font-mono text-xs leading-5 text-muted-foreground">
              {content}
            </pre>
          ) : kind === "task" && issue ? (
            <div className="mt-3 flex flex-col gap-3 pr-20">
              <div className="flex flex-wrap gap-2">
                <StatusBadge status={issue.status} />
                <Badge variant="outline">{issue.priority}</Badge>
                <Badge variant="outline">{turn.execution.kind}</Badge>
              </div>
              {issue.objective ? (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">
                    目标
                  </p>
                  <MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">
                    {issue.objective}
                  </MarkdownContent>
                </div>
              ) : null}
            </div>
          ) : kind === "comment" ? (
            <MarkdownContent className="mt-3 pr-20 text-[13px] !leading-5 [&>*+*]:!mt-2">
              {comment || visibleUserContent(content)}
            </MarkdownContent>
          ) : (
            <p className="mt-3 pr-20 text-[13px] leading-5 text-muted-foreground">
              {systemPromptSummary(content, kind)}
            </p>
          )}
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => setRaw((value) => !value)}
            className="absolute right-3 bottom-3 opacity-0 shadow-sm transition-opacity group-hover/prompt:opacity-100 group-focus-within/prompt:opacity-100"
          >
            {raw ? (
              <LayoutTemplate data-icon="inline-start" />
            ) : (
              <Code2 data-icon="inline-start" />
            )}
            {raw ? "渲染模式" : "原始模式"}
          </Button>
        </div>
      </MessageContent>
    </Message>
  )
}

function ProcessArea({
  execution,
  events,
  messages,
  active,
}: {
  execution: Execution
  events: ExecutionEvent[]
  messages: ChatMessage[]
  active: boolean
}) {
  const [open, setOpen] = React.useState(active)
  const [selectedEvent, setSelectedEvent] =
    React.useState<ExecutionEvent | null>(null)
  const processViewport = React.useRef<HTMLDivElement>(null)
  const processItems = React.useMemo(
    () =>
      [
        ...messages.map((message) => ({
          kind: "message" as const,
          id: message.id,
          createdAt: message.createdAt,
          message,
        })),
        ...events.map((event) => ({
          kind: "event" as const,
          id: event.id,
          createdAt: event.createdAt,
          event,
        })),
      ].sort(
        (left, right) =>
          left.createdAt.localeCompare(right.createdAt) ||
          left.id.localeCompare(right.id)
      ),
    [events, messages]
  )
  const signature = processItems
    .map((item) =>
      item.kind === "event"
        ? `${item.id}:${item.event.updatedAt ?? item.event.createdAt}:${item.event.status ?? ""}`
        : `${item.id}:${item.message.updatedAt}:${item.message.content.length}`
    )
    .join("|")

  React.useEffect(() => {
    if (!active || !open) return
    const viewport = processViewport.current
    if (viewport) viewport.scrollTop = viewport.scrollHeight
  }, [active, open, signature])

  if (processItems.length === 0) return null
  const elapsed = elapsedLabel(execution.startedAt, execution.finishedAt)
  const collapsedLabel = active
    ? `正在处理 ${elapsed} · ${processItems.length} 条过程消息`
    : `已处理 ${elapsed} · ${processItems.length} 条过程消息`

  return (
    <>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="flex w-full items-center gap-1.5 py-0.5 text-left text-xs leading-5 text-muted-foreground hover:text-foreground">
          {open ? (
            <ChevronDown className="size-4 shrink-0" />
          ) : (
            <ChevronRight className="size-4 shrink-0" />
          )}
          <span>
            {open
              ? `${active ? "正在执行" : "收起执行过程"} · ${processItems.length} 条`
              : collapsedLabel}
          </span>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div
            ref={processViewport}
            className="flex max-h-64 flex-col overflow-y-auto py-0.5 pr-1"
          >
            {processItems.map((item) =>
              item.kind === "message" ? (
                <MarkdownContent
                  key={item.id}
                  className="py-0.5 text-xs !leading-4 text-muted-foreground [&>*+*]:!mt-1"
                >
                  {item.message.content}
                </MarkdownContent>
              ) : (
                <button
                  type="button"
                  key={item.id}
                  onClick={() => setSelectedEvent(item.event)}
                  className="flex min-h-7 w-full items-center gap-2 rounded-md px-1.5 py-0.5 text-left text-xs leading-4 text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <TerminalSquare className="size-3.5 shrink-0" />
                  <span className="truncate">
                    {eventDescription(item.event, execution)}
                  </span>
                  {item.event.status ? (
                    <span className="ml-auto shrink-0 opacity-70">
                      {eventStatusLabel(item.event)}
                    </span>
                  ) : null}
                </button>
              )
            )}
          </div>
        </CollapsibleContent>
      </Collapsible>
      <EventDetailDialog
        key={selectedEvent?.id ?? "closed-event-detail"}
        event={selectedEvent}
        execution={execution}
        onClose={() => setSelectedEvent(null)}
      />
    </>
  )
}

function EventDetailDialog({
  event,
  execution,
  onClose,
}: {
  event: ExecutionEvent | null
  execution: Execution
  onClose: () => void
}) {
  const [fullEvent, setFullEvent] = React.useState<ExecutionEvent | null>(null)
  const [loading, setLoading] = React.useState(Boolean(event))
  const [error, setError] = React.useState("")

  React.useEffect(() => {
    if (!event) return
    let cancelled = false
    void fetchExecutionEvent(event.id)
      .then((next) => {
        if (!cancelled) setFullEvent(next)
      })
      .catch((reason) => {
        if (!cancelled)
          setError(reason instanceof Error ? reason.message : "读取事件详情失败")
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [event])

  const resolved = fullEvent ?? event
  return (
    <Dialog open={Boolean(event)} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="flex h-[520px] max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-2xl">
        <DialogHeader className="shrink-0 pr-8">
          <DialogTitle>
            {resolved ? eventDescription(resolved, execution) : "执行详情"}
          </DialogTitle>
          <DialogDescription>
            {resolved
              ? `${resolved.toolName || resolved.type} · ${eventStatusLabel(resolved)}`
              : "工具调用的输入、输出和执行状态"}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto rounded-lg bg-muted/60 p-4">
          {loading ? (
            <p className="text-xs text-muted-foreground">正在读取完整详情…</p>
          ) : error ? (
            <p className="text-xs text-destructive">{error}</p>
          ) : resolved ? (
            <div className="flex flex-col gap-4">
              {resolved.detail ? (
                <DetailSection label="说明" value={resolved.detail} />
              ) : null}
              {resolved.inputJson ? (
                <DetailSection label="输入" value={formatJSON(resolved.inputJson)} />
              ) : null}
              {resolved.outputJson ? (
                <DetailSection
                  label={resolved.isError ? "错误输出" : "输出"}
                  value={formatJSON(resolved.outputJson)}
                />
              ) : null}
              {!resolved.detail &&
              !resolved.inputJson &&
              !resolved.outputJson ? (
                <p className="text-xs text-muted-foreground">没有更多详情</p>
              ) : null}
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function DetailSection({ label, value }: { label: string; value: string }) {
  return (
    <section>
      <p className="mb-1.5 text-xs font-medium text-muted-foreground">
        {label}
      </p>
      <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs leading-5">
        {value}
      </pre>
    </section>
  )
}

function buildEmployeeTurns(
  executions: Execution[],
  messages: ChatMessage[],
  events: ExecutionEvent[],
  attachments: IssueAttachment[],
  issues: Issue[]
) {
  const issuesById = new Map(issues.map((issue) => [issue.id, issue]))
  const orderedExecutions = [...executions].sort(
    (left, right) =>
      left.startedAt.localeCompare(right.startedAt) ||
      left.id.localeCompare(right.id)
  )
  const result: EmployeeTurn[] = []

  for (const execution of orderedExecutions) {
    const executionMessages = messages
      .filter((message) => message.executionId === execution.id)
      .sort(
        (left, right) =>
          left.createdAt.localeCompare(right.createdAt) ||
          left.id.localeCompare(right.id)
      )
    const executionEvents = events
      .filter((event) => event.executionId === execution.id)
      .sort(
        (left, right) =>
          left.createdAt.localeCompare(right.createdAt) ||
          left.id.localeCompare(right.id)
      )
    const executionAttachments = attachments
      .filter((attachment) => attachment.executionId === execution.id)
      .sort(
        (left, right) =>
          left.createdAt.localeCompare(right.createdAt) ||
          left.id.localeCompare(right.id)
      )
    const turns: Array<
      Omit<EmployeeTurn, "active" | "processMessages" | "final"> & {
        outputs: ChatMessage[]
      }
    > = []
    let current: (typeof turns)[number] | undefined

    for (const message of executionMessages) {
      if (message.role === "user") {
        current = {
          id: message.id,
          user: message,
          outputs: [],
          events: [],
          attachments: [],
          execution,
          issue: issuesById.get(execution.issueId),
          startedAt: message.createdAt,
        }
        turns.push(current)
        continue
      }
      if (!current) {
        current = {
          id: `${execution.id}-output`,
          outputs: [],
          events: [],
          attachments: [],
          execution,
          issue: issuesById.get(execution.issueId),
          startedAt: message.createdAt,
        }
        turns.push(current)
      }
      current.outputs.push(message)
    }

    if (
      turns.length === 0 &&
      (executionEvents.length > 0 || executionAttachments.length > 0)
    ) {
      turns.push({
        id: `${execution.id}-activity`,
        outputs: [],
        events: [],
        attachments: [],
        execution,
        issue: issuesById.get(execution.issueId),
        startedAt: execution.startedAt,
      })
    }
    for (const event of executionEvents) {
      const target =
        [...turns]
          .reverse()
          .find((turn) => turn.startedAt <= event.createdAt) ?? turns[0]
      target?.events.push(event)
    }
    for (const attachment of executionAttachments) {
      const target =
        [...turns]
          .reverse()
          .find((turn) => turn.startedAt <= attachment.createdAt) ?? turns[0]
      target?.attachments.push(attachment)
    }

    turns.forEach((turn, index) => {
      const active =
        isExecutionActive(execution) && index === turns.length - 1
      const final = active ? undefined : turn.outputs[turn.outputs.length - 1]
      result.push({
        ...turn,
        active,
        final,
        processMessages: final ? turn.outputs.slice(0, -1) : turn.outputs,
      })
    })
  }
  return result
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  const units = ["KiB", "MiB", "GiB", "TiB"]
  let value = size / 1024
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${units[index]}`
}

function isExecutionActive(execution: Execution) {
  return ["queued", "starting", "running", "waiting_approval"].includes(
    execution.status
  )
}

function promptKind(turn: EmployeeTurn) {
  const content = turn.user?.content ?? ""
  if (/<comment>[\s\S]*?<\/comment>/.test(content)) return "comment" as const
  if (["planning", "work", "continuation", "rework", "recovery"].includes(turn.execution.kind))
    return "task" as const
  if (turn.execution.kind === "heartbeat") return "heartbeat" as const
  if (turn.execution.kind === "wakeup") return "system" as const
  return null
}

function extractComment(content: string) {
  return content.match(/<comment>\s*([\s\S]*?)\s*<\/comment>/)?.[1]?.trim() ?? ""
}

function systemPromptSummary(
  content: string,
  kind: "task" | "comment" | "heartbeat" | "system"
) {
  if (kind === "heartbeat") {
    const waitReason = content.match(/当前等待原因：([^\n]+)/)?.[1]?.trim()
    return waitReason
      ? `正在协调任务；当前等待原因：${waitReason}`
      : "正在检查任务状态并协调后续工作。"
  }
  return content.split("\n").find((line) => line.trim())?.trim() || "收到系统通知。"
}

function visibleUserContent(content: string) {
  return content
    .replace(
      /\s*<agent_permission_boundary>[\s\S]*?<\/agent_permission_boundary>\s*/g,
      ""
    )
    .trim()
}

function eventDescription(event: ExecutionEvent, execution: Execution) {
  const input = parseRecord(event.inputJson)
  const requestedDescription = stringValue(input.description)
  const snapshotDescription = execution.toolsSnapshot.find(
    (tool) => tool.name === event.toolName
  )?.description
  return (
    requestedDescription ||
    snapshotDescription ||
    event.title ||
    event.toolName ||
    "执行操作"
  )
}

function eventStatusLabel(event: ExecutionEvent) {
  if (event.isError || event.status === "failed") {
    const reason = eventErrorSummary(event)
    return reason ? `失败 · ${reason}` : "失败"
  }
  if (event.status === "completed") return "完成"
  if (event.status === "interrupted") return "已中断"
  if (event.status === "running") return "运行中"
  return event.status || event.type
}

function eventErrorSummary(event: ExecutionEvent) {
  const output = parseRecord(event.outputJson)
  const direct = stringValue(output.error || output.text)
  if (direct) return compactEventLabel(direct)
  const content = Array.isArray(output.content) ? output.content : []
  for (const part of content) {
    if (!part || typeof part !== "object" || Array.isArray(part)) continue
    const text = stringValue((part as Record<string, unknown>).text)
    if (text) return compactEventLabel(text)
  }
  return ""
}

function compactEventLabel(value: string) {
  const normalized = value.replace(/\s+/g, " ").trim()
  return normalized.length > 72 ? `${normalized.slice(0, 69)}…` : normalized
}

function parseRecord(value?: string) {
  if (!value) return {} as Record<string, unknown>
  try {
    const parsed: unknown = JSON.parse(value)
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : ""
}

function formatJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function elapsedLabel(start: string, finish?: string) {
  const elapsed = Math.max(
    0,
    new Date(finish ?? Date.now()).getTime() - new Date(start).getTime()
  )
  const seconds = Math.floor(elapsed / 1000)
  if (seconds >= 3600)
    return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  if (seconds >= 60) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${seconds}s`
}
