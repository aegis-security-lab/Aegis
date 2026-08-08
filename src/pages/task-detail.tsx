import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import { Archive, ChevronDown, Download, Paperclip } from "lucide-react"
import { useParams } from "react-router-dom"
import { toast } from "sonner"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { TimelinePanel } from "@/components/timeline-panel"
import { fetchTask } from "@/lib/api"
import { formatBytes, formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { TaskDetail } from "@/types"

export function TaskDetailPage() {
  const { taskId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<TaskDetail | null>(null)
  const [loading, setLoading] = React.useState(true)

  React.useEffect(() => {
    if (!taskId) return
    let active = true
    setLoading(true)
    void fetchTask(taskId)
      .then((next) => {
        if (active) setDetail(next)
      })
      .catch((reason) => {
        if (!active) return
        setDetail(null)
        toast.error(reason instanceof Error ? reason.message : "读取任务失败")
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [taskId])

  if (loading) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <Spinner />
      </div>
    )
  }
  if (!detail) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>任务不存在</EmptyTitle>
          <EmptyDescription>该地址没有对应的任务。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const task =
    state?.tasks.find((candidate) => candidate.id === detail.task.id) ??
    detail.task

  return (
    <div className="relative size-full min-h-0 overflow-hidden">
      <div className="size-full min-h-0 overflow-y-auto pr-1 lg:pr-[380px]">
        <div className="flex min-h-full flex-col gap-5">
          <header className="flex min-w-0 flex-wrap items-center gap-2 px-1">
            <ScrollingTitle value={task.title} />
            <Badge variant="outline">{task.priority}</Badge>
            <Badge variant="outline">
              人工验收：{task.humanValidationFallback ? "开启" : "关闭"}
            </Badge>
            <Badge variant="outline">
              预算：
              {task.timeBudgetMinutes
                ? `${task.timeBudgetMinutes} 分钟`
                : "不限"}
            </Badge>
            <Badge variant="secondary">
              {referenceName(
                task.assigneeAgentId,
                state?.agents.find(
                  (candidate) => candidate.id === task.assigneeAgentId
                )?.name
              )}
            </Badge>
          </header>

          <Card>
            <CardContent className="flex flex-col gap-5 py-5">
              <p className="text-sm leading-7 whitespace-pre-wrap">
                {task.description || "未填写任务描述"}
              </p>
              {task.objective ? (
                <div className="rounded-lg bg-muted/45 px-4 py-3 text-sm leading-6 whitespace-pre-wrap text-muted-foreground">
                  {task.objective}
                </div>
              ) : null}
            </CardContent>
          </Card>

          {detail.inputAttachments.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>输入附件</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                {detail.inputAttachments.map((attachment) => (
                  <div
                    key={attachment.id}
                    className="flex min-w-0 items-center gap-3 rounded-lg border px-3 py-2.5"
                  >
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                      <Paperclip className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">
                        {attachment.name}
                      </p>
                      <p className="truncate text-xs text-muted-foreground">
                        {attachment.mimeType} · {formatBytes(attachment.size)}
                      </p>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`下载 ${attachment.name}`}
                      title="下载附件"
                      render={
                        <a
                          href={`/api/input-attachments/${encodeURIComponent(attachment.id)}`}
                          download={attachment.name}
                        />
                      }
                      nativeButton={false}
                    >
                      <Download />
                    </Button>
                  </div>
                ))}
              </CardContent>
            </Card>
          ) : null}

          <Card>
            <CardHeader className="gap-1">
              <CardTitle>任务报告</CardTitle>
              <p className="text-xs leading-relaxed text-muted-foreground">
                每次根 Issue 完成后立即生成独立
                ZIP；默认下载最新版本，历史版本长期保留。
              </p>
            </CardHeader>
            <CardContent className="flex flex-col gap-1">
              {detail.rootReports.length > 0 ? (
                detail.rootReports.map((item) => {
                  const latest = item.reports[0]
                  const history = item.reports.slice(1)
                  return (
                    <div
                      key={item.issueId}
                      className="group flex min-w-0 items-center gap-3 rounded-lg px-3 py-3 transition-colors hover:bg-muted/45"
                    >
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                        <Archive className="size-4" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="flex min-w-0 items-baseline gap-2">
                          <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
                            {item.identifier}
                          </span>
                          <p
                            className="truncate text-sm font-medium"
                            title={item.title}
                          >
                            {item.title}
                          </p>
                        </div>
                        <p className="mt-0.5 truncate text-xs text-muted-foreground">
                          {latest
                            ? `${latest.title || item.title} · ${formatBytes(latest.size)} · 包含 ${latest.attachmentCount} 个附件`
                            : taskReportPlaceholder(item.status)}
                        </p>
                      </div>
                      {latest ? (
                        <div className="flex shrink-0 items-center">
                          <Button
                            variant="ghost"
                            size="sm"
                            className={
                              history.length ? "rounded-r-md pr-1.5" : undefined
                            }
                            render={
                              <a
                                href={`/api/task-reports/${encodeURIComponent(latest.id)}`}
                                download={latest.name}
                              />
                            }
                            nativeButton={false}
                          >
                            <Download />
                            下载最新
                          </Button>
                          {history.length ? (
                            <DropdownMenu>
                              <DropdownMenuTrigger
                                render={
                                  <Button
                                    variant="ghost"
                                    size="icon-sm"
                                    className="ml-0.5 rounded-l-md"
                                    aria-label="查看历史报告"
                                    title="查看历史报告"
                                  />
                                }
                              >
                                <ChevronDown />
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-80">
                                <DropdownMenuLabel>历史报告</DropdownMenuLabel>
                                {history.map((report) => (
                                  <DropdownMenuItem
                                    key={report.id}
                                    render={
                                      <a
                                        href={`/api/task-reports/${encodeURIComponent(report.id)}`}
                                        download={report.name}
                                      />
                                    }
                                  >
                                    <Archive />
                                    <span className="min-w-0 flex-1">
                                      <span
                                        className="block truncate text-xs font-medium"
                                        title={report.title}
                                      >
                                        {report.title || item.title}
                                      </span>
                                      <span className="block text-[11px] text-muted-foreground">
                                        v{report.version} ·{" "}
                                        {formatTime(report.createdAt)} ·{" "}
                                        {formatBytes(report.size)}
                                      </span>
                                    </span>
                                    <Download />
                                  </DropdownMenuItem>
                                ))}
                              </DropdownMenuContent>
                            </DropdownMenu>
                          ) : null}
                        </div>
                      ) : (
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {taskReportStatus(item.status)}
                        </span>
                      )}
                    </div>
                  )
                })
              ) : (
                <div className="rounded-lg bg-muted/35 px-4 py-6 text-center text-sm text-muted-foreground">
                  当前任务还没有根 Issue。
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
      <aside className="absolute inset-y-0 right-2 hidden w-[360px] flex-col overflow-hidden rounded-xl bg-card lg:flex">
        <TimelinePanel taskId={task.id} className="h-full" />
      </aside>
    </div>
  )
}

function taskReportPlaceholder(status: string) {
  if (status === "done" || status === "in_review") {
    return "交付已结束，最终报告尚未生成"
  }
  if (status === "cancelled") return "该根 Issue 已取消，不会生成最终报告"
  return "根 Issue 完成后将在这里生成最终报告 ZIP"
}

function taskReportStatus(status: string) {
  if (status === "done" || status === "in_review") return "待打包"
  if (status === "cancelled") return "已取消"
  return "执行中"
}

function referenceName(id?: string, name?: string) {
  if (!id) return "未指定"
  return name ? `${name} · ${id}` : id
}

function ScrollingTitle({ value }: { value: string }) {
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const textRef = React.useRef<HTMLHeadingElement>(null)
  const [offset, setOffset] = React.useState(0)
  return (
    <div
      ref={viewportRef}
      className="max-w-full min-w-48 flex-1 overflow-hidden"
      onMouseEnter={() => {
        const distance = Math.max(
          0,
          (textRef.current?.scrollWidth ?? 0) -
            (viewportRef.current?.clientWidth ?? 0)
        )
        setOffset(distance)
      }}
      onMouseLeave={() => setOffset(0)}
      title={value}
    >
      <h1
        ref={textRef}
        className="w-max max-w-none text-lg font-semibold tracking-tight transition-transform ease-linear"
        style={{
          transform: `translateX(-${offset}px)`,
          transitionDuration: offset
            ? `${Math.max(1.5, offset / 45)}s`
            : "200ms",
        }}
      >
        {value}
      </h1>
    </div>
  )
}
