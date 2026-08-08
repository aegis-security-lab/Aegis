import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Bot,
  CheckCircle2,
  ClipboardCheck,
  GitBranch,
  MessageSquareText,
  ShieldCheck,
  UserRound,
} from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { MarkdownContent } from "@/components/markdown-content"
import { StatusBadge } from "@/components/status-badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { fetchTaskTimeline } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  TaskTimeline,
  TaskTimelineEvent,
  TaskTimelineEventKind,
} from "@/types"

type TimelineFilter =
  "all" | "issue" | "execution" | "result" | "comment" | "approval" | "system"

const filters: Array<{ label: string; value: TimelineFilter }> = [
  { label: "全部", value: "all" },
  { label: "Issues", value: "issue" },
  { label: "Agent 执行", value: "execution" },
  { label: "结果与验收", value: "result" },
  { label: "评论", value: "comment" },
  { label: "审批", value: "approval" },
  { label: "系统", value: "system" },
]

export function TimelinePanel({
  taskId,
  className,
}: {
  taskId: string
  className?: string
}) {
  const { state } = useAppState()
  const [filter, setFilter] = React.useState<TimelineFilter>("all")
  const [timeline, setTimeline] = React.useState<TaskTimeline | null>(null)

  React.useEffect(() => {
    if (!taskId) return
    let current = true
    void fetchTaskTimeline(taskId)
      .then((result) => {
        if (current) setTimeline(result)
      })
      .catch((reason) => {
        if (current)
          toast.error(
            reason instanceof Error ? reason.message : "读取时间线失败"
          )
      })
    return () => {
      current = false
    }
  }, [taskId, state?.updatedAt])

  const visibleTimeline = timeline?.task.id === taskId ? timeline : null
  const events = React.useMemo(
    () =>
      (visibleTimeline?.events ?? []).filter((event) =>
        matchesFilter(event.kind, filter)
      ),
    [filter, visibleTimeline?.events]
  )

  return (
    <div className={cn("flex min-h-0 flex-col gap-3", className)}>
      <div className="flex min-w-0 shrink-0 items-center gap-3">
        <Select
          value={filter}
          onValueChange={(value) => {
            if (value) setFilter(value as TimelineFilter)
          }}
          aria-label="筛选时间线事件"
        >
          <SelectTrigger
            id="timeline-filter"
            size="sm"
            className="w-40"
            aria-label="筛选时间线事件"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {filters.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      {!visibleTimeline ? (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-32 w-full" />
          ))}
        </div>
      ) : events.length === 0 ? (
        <Empty className="min-h-0 flex-1 border-0">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Activity />
            </EmptyMedia>
            <EmptyTitle>没有匹配的关键事件</EmptyTitle>
            <EmptyDescription>
              切换筛选条件，或等待任务产生新的活动。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <VirtualTimeline
          key={`${taskId}:${filter}`}
          events={events}
          className="min-h-0 flex-1"
        />
      )}
    </div>
  )
}

function VirtualTimeline({
  events,
  className,
}: {
  events: TaskTimelineEvent[]
  className?: string
}) {
  const viewportRef = React.useRef<HTMLDivElement>(null)
  // TanStack Virtual owns an imperative measurement cache; React Compiler must
  // not memoize this hook's returned methods because their identity is stateful.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => viewportRef.current,
    estimateSize: () => 176,
    getItemKey: (index) => events[index]?.id ?? index,
    overscan: 6,
  })
  const virtualItems = virtualizer.getVirtualItems()

  const jump = (direction: -1 | 1) => {
    const visible = virtualizer.getVirtualItems()
    const current = visible.find(
      (item) => item.end > (virtualizer.scrollOffset ?? 0) + 8
    )
    const index = Math.min(
      events.length - 1,
      Math.max(0, (current?.index ?? 0) + direction)
    )
    virtualizer.scrollToIndex(index, { align: "start", behavior: "smooth" })
  }

  return (
    <div className={cn("relative min-h-0", className ?? "flex-1")}>
      <ScrollArea viewportRef={viewportRef} className="size-full">
        <section
          aria-label="任务关键事件"
          className="relative px-3 pt-2 pr-5"
          style={{ height: virtualizer.getTotalSize() + 8 }}
        >
          {virtualItems.map((virtualItem) => {
            const event = events[virtualItem.index]
            if (!event) return null
            return (
              <div
                key={event.id}
                ref={virtualizer.measureElement}
                data-index={virtualItem.index}
                className="absolute top-2 left-3 w-[calc(100%-2rem)]"
                style={{ transform: `translateY(${virtualItem.start}px)` }}
              >
                <TimelineItem
                  event={event}
                  last={virtualItem.index === events.length - 1}
                />
              </div>
            )
          })}
        </section>
      </ScrollArea>
      <div className="absolute right-4 bottom-4 z-10 flex flex-col gap-1.5">
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          className="rounded-full bg-background/90 shadow-md backdrop-blur"
          aria-label="上一个时间节点"
          title="上一个时间节点"
          onClick={() => jump(-1)}
        >
          <ArrowUp />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          className="rounded-full bg-background/90 shadow-md backdrop-blur"
          aria-label="下一个时间节点"
          title="下一个时间节点"
          onClick={() => jump(1)}
        >
          <ArrowDown />
        </Button>
      </div>
    </div>
  )
}

function TimelineItem({
  event,
  last,
}: {
  event: TaskTimelineEvent
  last: boolean
}) {
  const detail = event.detail?.trim()
  const summary = event.summary?.trim()
  const showDetail = detail && detail !== summary
  const content = showDetail ? detail : summary

  return (
    <article className="group relative min-w-0 pl-10">
      {!last ? (
        <span className="absolute top-8 bottom-0 left-4 w-px bg-border" />
      ) : null}
      <span className="absolute top-0 left-0 flex size-8 items-center justify-center rounded-full border bg-background text-muted-foreground">
        <EventIcon kind={event.kind} />
      </span>
      <div
        className={cn(
          "min-w-0 border-b border-border/60 pb-5",
          last && "border-b-0"
        )}
      >
        <div className="flex min-w-0 flex-col gap-1">
          <div className="flex min-w-0 items-start gap-2">
            <p className="min-w-0 flex-1 text-sm leading-5 font-medium">
              {event.title}
            </p>
            <span className="flex shrink-0 items-center gap-1.5">
              {event.status ? <StatusBadge status={event.status} /> : null}
              <time
                className="max-w-0 overflow-hidden text-[10px] leading-5 whitespace-nowrap text-muted-foreground opacity-0 transition-[max-width,opacity] group-hover:max-w-24 group-hover:opacity-100"
                title={formatTime(event.createdAt)}
              >
                {formatTime(event.createdAt)}
              </time>
            </span>
          </div>
          <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1">
              {event.actorType === "operator" ? (
                <UserRound className="size-3" />
              ) : (
                <Bot className="size-3" />
              )}
              {event.actorName}
            </span>
            <span>{event.issueIdentifier}</span>
          </p>
        </div>
        <div className="flex min-w-0 flex-col gap-3 pt-2">
          {summary ? (
            <MarkdownContent className="text-sm font-medium break-words">
              {summary}
            </MarkdownContent>
          ) : null}
          {content && content !== summary ? (
            <MarkdownContent className="min-w-0 text-sm text-muted-foreground">
              {content}
            </MarkdownContent>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Link
              to={"/issues/" + event.issueId}
              className={buttonVariants({ size: "xs", variant: "ghost" })}
            >
              查看 Issue
            </Link>
            {event.executionId ? (
              <Link
                to={"/sessions/" + event.executionId}
                className={buttonVariants({ size: "xs", variant: "ghost" })}
              >
                查看 Session
              </Link>
            ) : null}
          </div>
        </div>
      </div>
    </article>
  )
}

function EventIcon({ kind }: { kind: TaskTimelineEventKind }) {
  switch (kind) {
    case "issue":
      return <GitBranch className="size-4" />
    case "execution":
      return <Bot className="size-4" />
    case "result":
      return <CheckCircle2 className="size-4" />
    case "comment":
      return <MessageSquareText className="size-4" />
    case "validation":
      return <ShieldCheck className="size-4" />
    case "approval":
      return <ClipboardCheck className="size-4" />
    default:
      return <Activity className="size-4" />
  }
}

function matchesFilter(kind: TaskTimelineEventKind, filter: TimelineFilter) {
  if (filter === "all") return true
  if (filter === "result") return kind === "result" || kind === "validation"
  return kind === filter
}
