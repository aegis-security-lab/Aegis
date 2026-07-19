import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import {
  Activity,
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
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
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
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { fetchTaskTimeline } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
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

export function TimelinePage() {
  const { state } = useAppState()
  const tasks = React.useMemo(
    () =>
      [...(state?.issues ?? [])]
        .filter((issue) => !issue.parentId)
        .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)),
    [state?.issues]
  )
  const [taskID, setTaskID] = React.useState("")
  const [filter, setFilter] = React.useState<TimelineFilter>("all")
  const [timeline, setTimeline] = React.useState<TaskTimeline | null>(null)
  const resolvedTaskID = tasks.some((task) => task.id === taskID)
    ? taskID
    : (tasks[0]?.id ?? "")
  const visibleTimeline = timeline?.task.id === resolvedTaskID ? timeline : null

  React.useEffect(() => {
    if (!resolvedTaskID) return
    let current = true
    void fetchTaskTimeline(resolvedTaskID)
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
  }, [resolvedTaskID, state?.updatedAt])

  const taskItems = tasks.map((task) => ({
    label: task.identifier + " · " + task.title,
    value: task.id,
  }))
  const events = React.useMemo(
    () =>
      (visibleTimeline?.events ?? []).filter((event) =>
        matchesFilter(event.kind, filter)
      ),
    [filter, visibleTimeline?.events]
  )

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div className="flex flex-col gap-1">
            <CardTitle>任务时间线</CardTitle>
            <CardDescription>
              观察整个任务树的 Issue、Agent、结果、验收、评论与审批。
            </CardDescription>
          </div>
          <Field className="w-full lg:w-[480px]">
            <FieldLabel htmlFor="timeline-task">选择任务</FieldLabel>
            <Select
              items={taskItems}
              value={resolvedTaskID || null}
              onValueChange={(value) => setTaskID(value ?? "")}
              disabled={tasks.length === 0}
            >
              <SelectTrigger id="timeline-task" className="w-full">
                <SelectValue placeholder="选择一个顶层任务" />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {taskItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      <span className="max-w-[420px] truncate">
                        {item.label}
                      </span>
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription>
              时间线包含所选任务下任意层级的子 Issues。
            </FieldDescription>
          </Field>
        </CardHeader>
        {visibleTimeline ? (
          <CardContent className="flex flex-wrap items-center gap-2">
            <StatusBadge status={visibleTimeline.task.status} />
            <Badge variant="outline">{visibleTimeline.task.identifier}</Badge>
            <Badge variant="secondary">
              {visibleTimeline.issueCount} 个 Issues
            </Badge>
            <Badge variant="secondary">
              {visibleTimeline.events.length} 个关键事件
            </Badge>
          </CardContent>
        ) : null}
      </Card>

      {tasks.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Activity />
                </EmptyMedia>
                <EmptyTitle>还没有任务时间线</EmptyTitle>
                <EmptyDescription>
                  发布第一个任务后，关键事件会自动出现在这里。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="overflow-x-auto pb-1">
            <ToggleGroup
              value={[filter]}
              onValueChange={(value) => {
                if (value[0]) setFilter(value[0] as TimelineFilter)
              }}
              variant="outline"
              size="sm"
              aria-label="筛选时间线事件"
            >
              {filters.map((item) => (
                <ToggleGroupItem key={item.value} value={item.value}>
                  {item.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>

          {!visibleTimeline ? (
            <div className="flex flex-col gap-3">
              {Array.from({ length: 4 }).map((_, index) => (
                <Skeleton key={index} className="h-36 w-full" />
              ))}
            </div>
          ) : events.length === 0 ? (
            <Card>
              <CardContent className="py-14">
                <Empty>
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
              </CardContent>
            </Card>
          ) : (
            <VirtualTimeline
              key={`${resolvedTaskID}:${filter}`}
              events={events}
            />
          )}
        </>
      )}
    </div>
  )
}

function VirtualTimeline({ events }: { events: TaskTimelineEvent[] }) {
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

  return (
    <ScrollArea
      viewportRef={viewportRef}
      className="h-[min(720px,calc(100dvh-260px))] min-h-[420px] rounded-xl border bg-muted/10"
    >
      <section
        aria-label="任务关键事件"
        className="relative px-4 pt-4 pr-6"
        style={{ height: virtualizer.getTotalSize() + 16 }}
      >
        {virtualItems.map((virtualItem) => {
          const event = events[virtualItem.index]
          if (!event) return null
          return (
            <div
              key={event.id}
              ref={virtualizer.measureElement}
              data-index={virtualItem.index}
              className="absolute top-4 left-4 w-[calc(100%-2.5rem)]"
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
    <article className="grid grid-cols-[32px_minmax(0,1fr)] gap-3 pb-4">
      <div className="relative flex justify-center">
        {!last ? (
          <span className="absolute top-8 bottom-0 w-px bg-border" />
        ) : null}
        <span className="relative flex size-8 items-center justify-center rounded-full border bg-background text-muted-foreground">
          <EventIcon kind={event.kind} />
        </span>
      </div>
      <Card className="py-4">
        <CardHeader className="px-4">
          <div className="flex min-w-0 flex-col gap-1">
            <CardTitle className="text-sm leading-5">{event.title}</CardTitle>
            <CardDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span className="inline-flex items-center gap-1">
                {event.actorType === "operator" ? (
                  <UserRound className="size-3" />
                ) : (
                  <Bot className="size-3" />
                )}
                {event.actorName}
              </span>
              <span>{event.issueIdentifier}</span>
              <span>{formatTime(event.createdAt)}</span>
            </CardDescription>
          </div>
          <CardAction className="flex items-center gap-2">
            {event.status ? <StatusBadge status={event.status} /> : null}
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 px-4">
          {summary ? (
            <p className="text-sm font-medium break-words">{summary}</p>
          ) : null}
          {content && content !== summary ? (
            content.length > 500 ? (
              <ScrollArea className="h-36 rounded-lg border bg-muted/20 p-3">
                <MarkdownContent className="pr-3 text-sm">
                  {content}
                </MarkdownContent>
              </ScrollArea>
            ) : (
              <MarkdownContent className="text-sm text-muted-foreground">
                {content}
              </MarkdownContent>
            )
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              size="xs"
              variant="ghost"
              render={<Link to={"/issues/" + event.issueId} />}
              nativeButton={false}
            >
              查看 Issue
            </Button>
            {event.executionId ? (
              <Button
                size="xs"
                variant="ghost"
                render={<Link to={"/sessions/" + event.executionId} />}
                nativeButton={false}
              >
                查看 Session
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>
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
