import * as React from "react"
import {
  Bot,
  Check,
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  ClipboardCheck,
  Copy,
  LoaderCircle,
  TerminalSquare,
} from "lucide-react"

import { ChatMessage as ChatMessageView } from "@/components/chat-message"
import { MarkdownContent } from "@/components/markdown-content"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
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
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { fetchExecutionEvent } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type {
  Approval,
  Execution,
  ExecutionEvent,
  IssueValidation,
  Message as ChatMessage,
} from "@/types"

type ActivityItem =
  | { kind: "message"; id: string; createdAt: string; message: ChatMessage }
  | { kind: "event"; id: string; createdAt: string; event: ExecutionEvent }
  | {
      kind: "validation"
      id: string
      createdAt: string
      validation: IssueValidation
    }
  | { kind: "approval"; id: string; createdAt: string; approval: Approval }

type ActivityGroup =
  | { kind: "pinned"; id: string; item: ActivityItem }
  | { kind: "process"; id: string; items: ActivityItem[] }

export function IssueAgentActivity({
  executions,
  messages,
  events,
  validations,
  approvals,
  className,
}: {
  executions: Execution[]
  messages: ChatMessage[]
  events: ExecutionEvent[]
  validations: IssueValidation[]
  approvals: Approval[]
  className?: string
}) {
  const [selected, setSelected] = React.useState<ActivityItem | null>(null)
  const executionMap = React.useMemo(
    () => new Map(executions.map((execution) => [execution.id, execution])),
    [executions]
  )
  const items = React.useMemo<ActivityItem[]>(
    () =>
      [
        ...messages
          .filter((message) => message.role === "assistant" && message.content)
          .map((message) => ({
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
        ...validations.map((validation) => ({
          kind: "validation" as const,
          id: validation.id,
          createdAt: validation.completedAt ?? validation.createdAt,
          validation,
        })),
        ...approvals.map((approval) => ({
          kind: "approval" as const,
          id: approval.id,
          createdAt: approval.resolvedAt ?? approval.createdAt,
          approval,
        })),
      ].sort(
        (left, right) =>
          left.createdAt.localeCompare(right.createdAt) ||
          left.id.localeCompare(right.id)
      ),
    [approvals, events, messages, validations]
  )
  const groups = React.useMemo<ActivityGroup[]>(() => {
    const grouped: ActivityGroup[] = []
    for (const item of items) {
      if (item.kind !== "event") {
        grouped.push({ kind: "pinned", id: item.id, item })
        continue
      }
      const previous = grouped.at(-1)
      if (previous?.kind === "process") previous.items.push(item)
      else grouped.push({ kind: "process", id: item.id, items: [item] })
    }
    return grouped
  }, [items])
  const active = executions.some((execution) =>
    ["queued", "starting", "running", "waiting_approval"].includes(
      execution.status
    )
  )
  const lastGroup = groups.at(-1)
  const activeGroup = Boolean(active && lastGroup?.kind === "process")

  return (
    <>
      <MessageScrollerProvider defaultScrollPosition="end" autoScroll={active}>
        <MessageScroller className={cn("min-h-0 flex-1", className)}>
          <MessageScrollerViewport className="h-full min-w-0 overflow-x-hidden px-3 py-3">
            <MessageScrollerContent className="gap-2.5">
              {groups.length === 0 ? (
                <div className="m-auto flex max-w-56 flex-col items-center gap-2 text-center text-sm text-muted-foreground">
                  <Bot />
                  Agent 开始工作后，对话、工具调用和验收决策会实时显示在这里。
                </div>
              ) : null}
              {groups.map((group) => (
                <ActivityGroupRow
                  key={group.id}
                  group={group}
                  active={activeGroup && group === lastGroup}
                  executionMap={executionMap}
                  onSelect={setSelected}
                />
              ))}
            </MessageScrollerContent>
          </MessageScrollerViewport>
          <MessageScrollerButton />
        </MessageScroller>
      </MessageScrollerProvider>
      {selected ? (
        <ActivityDetailDialog
          key={`${selected.kind}-${selected.id}`}
          item={selected}
          onClose={() => setSelected(null)}
        />
      ) : null}
    </>
  )
}

function ActivityGroupRow({
  group,
  active,
  executionMap,
  onSelect,
}: {
  group: ActivityGroup
  active: boolean
  executionMap: Map<string, Execution>
  onSelect: (item: ActivityItem) => void
}) {
  if (group.kind === "pinned") {
    const item = group.item
    if (item.kind === "message") {
      const execution = executionMap.get(item.message.executionId)
      return (
        <MessageScrollerItem messageId={item.id}>
          <ChatMessageView
            message={item.message}
            label={execution?.kind ?? "Agent"}
            time={formatTime(item.createdAt)}
            onSelect={() => onSelect(item)}
            compact
          />
        </MessageScrollerItem>
      )
    }
    const presentation = activityPresentation(item)
    return (
      <MessageScrollerItem messageId={item.id}>
        <Marker
          render={
            <button
              type="button"
              onClick={() => onSelect(item)}
              className="rounded-lg px-2 py-1.5 hover:bg-muted hover:text-foreground"
            />
          }
        >
          <MarkerIcon>{presentation.icon}</MarkerIcon>
          <MarkerContent className="flex min-w-0 flex-1 items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-xs font-medium">
              {presentation.title}
            </span>
            {presentation.status ? (
              <StatusBadge status={presentation.status} />
            ) : null}
            <time className="shrink-0 text-[11px]">
              {formatTime(item.createdAt)}
            </time>
          </MarkerContent>
        </Marker>
      </MessageScrollerItem>
    )
  }
  return (
    <MessageScrollerItem messageId={group.items.at(-1)!.id}>
      <AgentProcess
        items={group.items.filter(
          (item): item is ActivityItem & { kind: "event" } =>
            item.kind === "event"
        )}
        active={active}
        onSelect={onSelect}
      />
    </MessageScrollerItem>
  )
}

function AgentProcess({
  items,
  active,
  onSelect,
}: {
  items: Array<ActivityItem & { kind: "event" }>
  active: boolean
  onSelect: (item: ActivityItem) => void
}) {
  const [historyOpen, setHistoryOpen] = React.useState(false)
  const [fullEvents, setFullEvents] = React.useState<
    Map<string, ExecutionEvent>
  >(new Map())
  const open = active || historyOpen

  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    const toolEvents = items.filter(
      (item) => item.event.type === "tool" && !fullEvents.has(item.event.id)
    )
    if (toolEvents.length === 0) return
    void Promise.all(
      toolEvents.map((item) =>
        fetchExecutionEvent(item.event.id).catch(() => undefined)
      )
    ).then((events) => {
      if (cancelled) return
      const next = new Map(fullEvents)
      for (const event of events) {
        if (event) next.set(event.id, event)
      }
      setFullEvents(next)
    })
    return () => {
      cancelled = true
    }
  }, [open, items, fullEvents])

  const processViewport = React.useRef<HTMLDivElement>(null)
  React.useEffect(() => {
    if (active && processViewport.current) {
      processViewport.current.scrollTop = processViewport.current.scrollHeight
    }
  }, [active, items])

  return (
    <Collapsible
      open={open}
      onOpenChange={(value) => {
        if (!active) setHistoryOpen(value)
      }}
      className="ml-3 min-w-0"
    >
      <CollapsibleTrigger
        render={<Button variant="ghost" size="xs" />}
        className="text-muted-foreground"
      >
        {active ? (
          <LoaderCircle className="animate-spin" />
        ) : (
          <ChevronDown
            className={cn("transition-transform", open && "rotate-180")}
          />
        )}
        {active ? "Agent 正在执行" : `查看执行过程 · ${items.length} 条`}
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-1 overflow-hidden data-[ending-style]:animate-out data-[starting-style]:animate-in">
        <div
          ref={processViewport}
          className="flex max-h-56 flex-col gap-2 overflow-y-auto py-1 pr-1 pl-1"
        >
          {items.map((item) => (
            <ToolEventRow
              key={item.id}
              item={item}
              fullEvent={fullEvents.get(item.event.id)}
              onSelect={() => onSelect(item)}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function ToolEventRow({
  item,
  fullEvent,
  onSelect,
}: {
  item: ActivityItem & { kind: "event" }
  fullEvent: ExecutionEvent | undefined
  onSelect: () => void
}) {
  const event = item.event
  const isTool = event.type === "tool"
  const summary = eventSummary(event)
  const row = (
    <button
      type="button"
      onClick={isTool ? undefined : onSelect}
      className="flex w-full min-w-0 items-start gap-2 rounded-md text-left text-xs leading-5 text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      {event.isError ? (
        <CircleAlert className="mt-0.5 size-3.5 shrink-0 text-destructive" />
      ) : (
        <TerminalSquare className="mt-0.5 size-3.5 shrink-0" />
      )}
      {isTool ? (
        <span
          className="min-w-0 flex-1 [scrollbar-width:none] overflow-x-auto font-mono whitespace-nowrap [&::-webkit-scrollbar]:hidden"
          title={summary}
          onWheel={(event) => {
            event.currentTarget.scrollLeft += event.deltaY || event.deltaX
          }}
        >
          {summary}
        </span>
      ) : (
        <span className="min-w-0 flex-1 truncate">{summary}</span>
      )}
    </button>
  )
  if (!isTool) return <div className="min-w-0">{row}</div>
  return (
    <Popover>
      <PopoverTrigger render={row} />
      <PopoverContent
        side="left"
        align="start"
        sideOffset={10}
        className="w-[min(34rem,calc(100vw-2rem))] gap-0 overflow-hidden p-0"
      >
        <ToolEventInspector event={fullEvent ?? event} />
      </PopoverContent>
    </Popover>
  )
}

function ToolEventInspector({ event }: { event: ExecutionEvent }) {
  const input = meaningfulJSON(event.inputJson) || event.detail
  const output = event.outputJson
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2 border-b px-3 py-2.5">
        <TerminalSquare className="size-3.5 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate font-mono text-xs font-medium">
          {toolDisplayName(event)}
        </span>
        {event.status ? <StatusBadge status={event.status} /> : null}
      </div>
      <div className="max-h-[min(30rem,70vh)] overflow-y-auto p-3">
        {input ? <ToolValue label="参数" value={formatJSON(input)} /> : null}
        {output ? (
          <ToolValue
            label={event.isError ? "错误输出" : "输出"}
            value={formatJSON(output)}
            error={event.isError}
            className={input ? "mt-3" : undefined}
          />
        ) : null}
        {!input && !output ? (
          <p className="text-xs text-muted-foreground">没有可展示的详细信息</p>
        ) : null}
      </div>
    </div>
  )
}

function ToolValue({
  label,
  value,
  error = false,
  className,
}: {
  label: string
  value: string
  error?: boolean
  className?: string
}) {
  const [copied, setCopied] = React.useState(false)
  const copy = async () => {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }
  return (
    <section className={className}>
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <span className="text-[11px] font-medium text-muted-foreground">
          {label}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => void copy()}
          aria-label={`复制${label}`}
        >
          {copied ? <Check /> : <Copy />}
          {copied ? "已复制" : "复制"}
        </Button>
      </div>
      <pre
        className={cn(
          "max-h-64 overflow-auto rounded-md bg-muted/55 p-2.5 font-mono text-[11px] leading-5 break-words whitespace-pre-wrap",
          error && "text-destructive"
        )}
      >
        {value}
      </pre>
    </section>
  )
}

function activityPresentation(
  item: Exclude<ActivityItem, { kind: "message" }>
) {
  if (item.kind === "validation") {
    return {
      title: `验收决策 · ${item.validation.summary || validationLabel(item.validation)}`,
      status: item.validation.status,
      icon: item.validation.passed ? <CheckCircle2 /> : <ClipboardCheck />,
    }
  }
  if (item.kind === "approval") {
    return {
      title: `${item.approval.title || "审批决策"} · ${item.approval.detail || item.approval.type}`,
      status: item.approval.status,
      icon: <ClipboardCheck />,
    }
  }
  return {
    title: eventSummary(item.event),
    status: item.event.status,
    icon: item.event.isError ? <CircleAlert /> : <TerminalSquare />,
  }
}

function ActivityDetailDialog({
  item,
  onClose,
}: {
  item: ActivityItem
  onClose: () => void
}) {
  const [fullEvent, setFullEvent] = React.useState<ExecutionEvent | null>(null)
  const [loading, setLoading] = React.useState(item.kind === "event")
  const [error, setError] = React.useState("")

  React.useEffect(() => {
    if (item.kind !== "event") return
    let cancelled = false
    void fetchExecutionEvent(item.event.id)
      .then((event) => !cancelled && setFullEvent(event))
      .catch(
        (reason) =>
          !cancelled &&
          setError(reason instanceof Error ? reason.message : "读取详情失败")
      )
      .finally(() => !cancelled && setLoading(false))
    return () => {
      cancelled = true
    }
  }, [item])

  const title = item ? activityTitle(item) : "AI 活动详情"
  return (
    <Dialog open={Boolean(item)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-3xl">
        <DialogHeader className="shrink-0 pr-8">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            完整展示 Agent 对话、工具输入输出或验收决策证据。
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto rounded-lg bg-muted/50 p-4">
          {loading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner />
              正在读取完整事件…
            </div>
          ) : error ? (
            <p className="text-sm text-destructive">{error}</p>
          ) : item ? (
            <ActivityDetail item={item} fullEvent={fullEvent} />
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ActivityDetail({
  item,
  fullEvent,
}: {
  item: ActivityItem
  fullEvent: ExecutionEvent | null
}) {
  if (item.kind === "message") {
    return (
      <DetailSection label="Agent 完整回复">
        <MarkdownContent className="text-sm !leading-6">
          {item.message.content}
        </MarkdownContent>
      </DetailSection>
    )
  }
  if (item.kind === "validation") {
    const validation = item.validation
    return (
      <div className="flex flex-col gap-5">
        <DetailSection
          label="决策摘要"
          value={validation.summary || "未填写"}
        />
        <DetailSection label="验收目标" value={validation.objective} />
        <DetailSection label="候选交付" value={validation.candidateResult} />
        {validation.feedback ? (
          <DetailSection label="反馈" value={validation.feedback} />
        ) : null}
        {validation.abandonmentProof ? (
          <DetailSection label="放弃证明" value={validation.abandonmentProof} />
        ) : null}
        {validation.error ? (
          <DetailSection label="错误" value={validation.error} />
        ) : null}
        <DetailSection
          label="状态"
          value={`${validation.status} · attempt ${validation.attempt}`}
        />
      </div>
    )
  }
  if (item.kind === "approval") {
    return (
      <div className="flex flex-col gap-5">
        <DetailSection label="审批内容" value={item.approval.detail} />
        <DetailSection label="类型" value={item.approval.type} />
        <DetailSection label="状态" value={item.approval.status} />
      </div>
    )
  }
  const event = fullEvent ?? item.event
  return (
    <div className="flex flex-col gap-5">
      {event.detail ? (
        <DetailSection label="说明" value={event.detail} />
      ) : null}
      {event.inputJson ? (
        <DetailSection
          label="完整输入"
          value={formatJSON(event.inputJson)}
          mono
        />
      ) : null}
      {event.outputJson ? (
        <DetailSection
          label={event.isError ? "完整错误输出" : "完整输出"}
          value={formatJSON(event.outputJson)}
          mono
        />
      ) : null}
      {!event.detail && !event.inputJson && !event.outputJson ? (
        <DetailSection label="事件" value={event.title} />
      ) : null}
    </div>
  )
}

function DetailSection({
  label,
  value,
  mono = false,
  children,
}: {
  label: string
  value?: string
  mono?: boolean
  children?: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-1.5">
      <h3 className="text-xs font-medium text-muted-foreground">{label}</h3>
      {children ??
        (mono ? (
          <pre className="overflow-x-auto font-mono text-xs leading-5 break-words whitespace-pre-wrap">
            {value}
          </pre>
        ) : (
          <MarkdownContent className="text-sm !leading-6">
            {value || "未填写"}
          </MarkdownContent>
        ))}
    </section>
  )
}

function activityTitle(item: ActivityItem) {
  if (item.kind === "message") return "Agent 对话详情"
  if (item.kind === "validation") return "验收决策详情"
  if (item.kind === "approval") return item.approval.title || "审批详情"
  return eventSummary(item.event)
}

function eventSummary(event: ExecutionEvent) {
  if (event.type === "tool") {
    const name = toolDisplayName(event)
    const detail = toolSummaryDetail(event)
    return detail ? `${name} · ${detail}` : name
  }
  return event.detail
    ? `${event.title} · ${oneLine(event.detail)}`
    : event.title
}

function toolDisplayName(event: ExecutionEvent) {
  const raw = (event.toolName || event.title || "Tool")
    .replace(/^工具调用\s*[·:：-]?\s*/i, "")
    .replace(/^Go\s+AgentCore\s+调用\s*/i, "")
    .replace(/^AgentCore\s+调用\s*/i, "")
    .trim()
  const names: Record<string, string> = {
    bash: "Bash",
    read: "Read",
    write: "Write",
    edit: "Edit",
    glob: "Glob",
    grep: "Grep",
  }
  return names[raw.toLowerCase()] ?? raw
}

function toolSummaryDetail(event: ExecutionEvent) {
  const raw = event.detail || meaningfulJSON(event.inputJson)
  if (!raw) return ""
  try {
    const value = JSON.parse(raw) as Record<string, unknown>
    for (const key of [
      "command",
      "path",
      "file_path",
      "query",
      "pattern",
      "url",
    ]) {
      if (typeof value[key] === "string" && value[key]) {
        return oneLine(value[key])
      }
    }
  } catch {
    // Plain-text tool arguments are already suitable for a compact summary.
  }
  return oneLine(raw)
}

function meaningfulJSON(value?: string) {
  const normalized = value?.trim()
  return normalized && normalized !== "{}" ? normalized : ""
}

function validationLabel(validation: IssueValidation) {
  if (validation.passed) return "通过"
  if (validation.status === "abandoned") return "目标放弃"
  if (validation.feedback) return "要求返工"
  return validation.status
}

function oneLine(value: string) {
  return value.replace(/\s+/g, " ").trim().slice(0, 100)
}

function formatJSON(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}
