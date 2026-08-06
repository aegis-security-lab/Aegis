import * as React from "react"
import {
  Bot,
  CheckCircle2,
  CircleAlert,
  ClipboardCheck,
  TerminalSquare,
} from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { StatusBadge } from "@/components/status-badge"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
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
  const active = executions.some((execution) =>
    ["queued", "starting", "running", "waiting_approval"].includes(
      execution.status
    )
  )

  return (
    <>
      <MessageScrollerProvider defaultScrollPosition="end" autoScroll={active}>
        <MessageScroller className={cn("h-[34rem] min-h-0", className)}>
          <MessageScrollerViewport className="h-full px-3 py-3">
            <MessageScrollerContent className="gap-2.5">
              {items.length === 0 ? (
                <div className="m-auto flex max-w-56 flex-col items-center gap-2 text-center text-sm text-muted-foreground">
                  <Bot />
                  Agent 开始工作后，对话、工具调用和验收决策会实时显示在这里。
                </div>
              ) : null}
              {items.map((item) => (
                <ActivityRow
                  key={`${item.kind}-${item.id}`}
                  item={item}
                  execution={
                    item.kind === "message"
                      ? executionMap.get(item.message.executionId)
                      : item.kind === "event"
                        ? executionMap.get(item.event.executionId)
                        : item.kind === "approval"
                          ? executionMap.get(item.approval.executionId)
                          : executionMap.get(item.validation.sourceExecutionId)
                  }
                  onSelect={() => setSelected(item)}
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

function ActivityRow({
  item,
  execution,
  onSelect,
}: {
  item: ActivityItem
  execution?: Execution
  onSelect: () => void
}) {
  if (item.kind === "message") {
    return (
      <MessageScrollerItem messageId={item.id}>
        <Message>
          <MessageContent>
            <MessageHeader>
              {execution?.kind ?? "Agent"} · {formatTime(item.createdAt)}
            </MessageHeader>
            <Bubble variant="muted">
              <BubbleContent
                render={<button type="button" onClick={onSelect} />}
              >
                <MarkdownContent className="line-clamp-5 text-xs !leading-5 [&>*+*]:!mt-1.5">
                  {item.message.content}
                </MarkdownContent>
              </BubbleContent>
            </Bubble>
          </MessageContent>
        </Message>
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
            onClick={onSelect}
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
    return `${event.toolName || "工具调用"} · ${event.title}`
  }
  return event.detail
    ? `${event.title} · ${oneLine(event.detail)}`
    : event.title
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
