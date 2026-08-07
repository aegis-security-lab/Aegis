import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  Download,
  Paperclip,
} from "lucide-react"
import { useParams } from "react-router-dom"
import { toast } from "sonner"

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Button } from "@/components/ui/button"
import { TimelinePanel } from "@/components/timeline-panel"
import { fetchIssue } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { IssueDetail } from "@/types"

export function TaskDetailPage() {
  const { issueId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [loading, setLoading] = React.useState(true)

  React.useEffect(() => {
    if (!issueId) return
    let active = true
    setLoading(true)
    void fetchIssue(issueId)
      .then((next) => {
        if (active) setDetail(next)
      })
      .catch((reason) => {
        if (!active) return
        setDetail(null)
        toast.error(
          reason instanceof Error ? reason.message : "读取任务失败"
        )
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [issueId])

  if (loading) {
    return (
      <div className="flex min-h-64 items-center justify-center">
        <Spinner />
      </div>
    )
  }
  if (!detail || detail.issue.parentId) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>任务不存在</EmptyTitle>
          <EmptyDescription>
            该地址没有对应的顶层任务执行记录。
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  const issue =
    state?.issues.find((candidate) => candidate.id === detail.issue.id) ??
    detail.issue
  const source = state?.tasks.find(
    (candidate) => candidate.id === issue.taskSourceId
  )
  const parameters = source ?? issue

  return (
    <div className="relative size-full min-h-0 overflow-hidden">
      <div className="size-full min-h-0 overflow-y-auto pr-1 lg:pr-[380px]">
        <div className="flex min-h-full flex-col gap-5">
          <Card>
            <CardContent className="grid gap-x-8 gap-y-5 text-sm sm:grid-cols-2">
              <TaskParameter label="标题" value={parameters.title} />
              <TaskParameter
                label="描述"
                value={parameters.description || "未填写"}
              />
              <TaskParameter
                label="目标"
                value={parameters.objective || "未填写"}
              />
              <TaskParameter
                label="执行边界"
                value={parameters.constraints || "未填写"}
              />
              <TaskParameter
                label="负责 Agent"
                value={referenceName(
                  parameters.assigneeAgentId,
                  state?.agents.find(
                    (candidate) =>
                      candidate.id === parameters.assigneeAgentId
                  )?.name
                )}
              />
              <TaskParameter label="优先级" value={parameters.priority} />
              <TaskParameter
                label="时间预算（分钟）"
                value={
                  parameters.timeBudgetMinutes
                    ? String(parameters.timeBudgetMinutes)
                    : "不限制"
                }
              />
              <TaskParameter
                label="人工兜底验收"
                value={
                  parameters.humanValidationFallback ? "启用" : "关闭"
                }
              />
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
        </div>
      </div>
      <aside className="absolute inset-y-0 right-2 hidden w-[360px] flex-col overflow-hidden rounded-xl bg-card lg:flex">
        <TimelinePanel taskId={issue.id} className="h-full" />
      </aside>
    </div>
  )
}

function referenceName(id?: string, name?: string) {
  if (!id) return "未指定"
  return name ? `${name} · ${id}` : id
}

function TaskParameter({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p
        className={cn(
          "mt-1 leading-relaxed break-all whitespace-pre-wrap",
          mono && "font-mono text-xs"
        )}
      >
        {value}
      </p>
    </div>
  )
}

