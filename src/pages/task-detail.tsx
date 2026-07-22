import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import { File, Folder, LinkIcon } from "lucide-react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"

import { StatusBadge } from "@/components/status-badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import { fetchIssue, fetchTaskWorkspace } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, IssueDetail, Task, TaskWorkspace } from "@/types"

export function TaskDetailPage() {
  const { issueId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [workspace, setWorkspace] = React.useState<TaskWorkspace | null>(null)
  const [workspaceError, setWorkspaceError] = React.useState("")
  const [loading, setLoading] = React.useState(true)

  React.useEffect(() => {
    if (!issueId) return
    let active = true
    setLoading(true)
    void Promise.allSettled([
      fetchIssue(issueId),
      fetchTaskWorkspace(issueId),
    ]).then(([issueResult, workspaceResult]) => {
      if (!active) return
      if (issueResult.status === "fulfilled") {
        setDetail(issueResult.value)
      } else {
        setDetail(null)
        toast.error(
          issueResult.reason instanceof Error
            ? issueResult.reason.message
            : "读取任务失败"
        )
      }
      if (workspaceResult.status === "fulfilled") {
        setWorkspace(workspaceResult.value)
        setWorkspaceError("")
      } else {
        setWorkspace(null)
        setWorkspaceError(
          workspaceResult.reason instanceof Error
            ? workspaceResult.reason.message
            : "读取工作区失败"
        )
      }
      setLoading(false)
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
  const runs = taskRuns(issue, source, state?.issues ?? [issue])

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>原始任务参数</CardTitle>
          <CardDescription>
            发布任务时保存的参数；重新执行会从这份定义创建新的根 Issue。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-x-8 gap-y-5 text-sm sm:grid-cols-2">
          <TaskParameter label="标题" value={parameters.title} />
          <TaskParameter
            label="项目"
            value={referenceName(
              parameters.projectId,
              state?.projects.find(
                (candidate) => candidate.id === parameters.projectId
              )?.name
            )}
          />
          <TaskParameter
            label="描述"
            value={parameters.description || "未填写"}
          />
          <TaskParameter
            label="背景与上下文"
            value={parameters.context || "未填写"}
          />
          <TaskParameter
            label="目标"
            value={parameters.objective || "未填写"}
          />
          <TaskParameter
            label="执行边界"
            value={parameters.constraints || "未填写"}
          />
          <TaskParameter label="工作目录" value={parameters.workspace} mono />
          <TaskParameter
            label="执行环境"
            value={
              parameters.containerProfileId
                ? referenceName(
                    parameters.containerProfileId,
                    state?.containerProfiles.find(
                      (candidate) =>
                        candidate.id === parameters.containerProfileId
                    )?.name
                  )
                : "宿主机"
            }
          />
          <TaskParameter
            label="负责 Agent"
            value={referenceName(
              parameters.assigneeAgentId,
              state?.agents.find(
                (candidate) => candidate.id === parameters.assigneeAgentId
              )?.name
            )}
          />
          <TaskParameter
            label="工作模式"
            value={parameters.workMode === "guided" ? "引导模式" : "自治模式"}
          />
          <TaskParameter label="优先级" value={parameters.priority} />
          <TaskParameter
            label="创建时间"
            value={formatTime(parameters.createdAt)}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>执行记录</CardTitle>
          <CardDescription>
            每次执行都会创建独立的根 Issue；进入记录可查看该次执行的 Issue
            协作过程。
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {runs.map((run, index) => (
            <Link
              key={run.id}
              to={`/issues/${run.id}`}
              className="flex items-center gap-4 rounded-lg border px-4 py-3 transition-colors hover:bg-muted/40"
            >
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    第 {runs.length - index} 次
                  </span>
                  <span className="truncate text-sm font-medium">
                    {run.identifier} · {run.title}
                  </span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {formatTime(run.createdAt)} ·{" "}
                  {agentLabel(run, state?.agents ?? [])}
                </span>
              </div>
              <StatusBadge status={run.status} />
              <LinkIcon className="size-4 shrink-0 text-muted-foreground" />
            </Link>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>工作区文件</CardTitle>
          <CardDescription className="break-all">
            {workspace?.root ?? parameters.workspace}
            {workspace ? ` · ${workspace.entries.length} 项` : ""}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {workspaceError ? (
            <p className="text-sm text-destructive">{workspaceError}</p>
          ) : workspace ? (
            <ScrollArea className="h-[420px] rounded-md border">
              <div className="divide-y text-sm">
                {workspace.entries.map((entry) => (
                  <div
                    key={entry.path}
                    className="flex items-center gap-3 px-4 py-2.5"
                  >
                    {entry.kind === "directory" ? (
                      <Folder className="size-4 shrink-0 text-muted-foreground" />
                    ) : (
                      <File className="size-4 shrink-0 text-muted-foreground" />
                    )}
                    <span className="min-w-0 flex-1 font-mono text-xs break-all">
                      {entry.path}
                    </span>
                    {entry.kind === "file" ? (
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {formatBytes(entry.size ?? 0)}
                      </span>
                    ) : null}
                  </div>
                ))}
                {workspace.entries.length === 0 ? (
                  <p className="p-4 text-muted-foreground">工作区为空</p>
                ) : null}
              </div>
            </ScrollArea>
          ) : (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner />
              正在读取工作区…
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function taskRuns(issue: Issue, source: Task | undefined, issues: Issue[]) {
  if (!source) return [issue]
  return issues
    .filter(
      (candidate) => !candidate.parentId && candidate.taskSourceId === source.id
    )
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt))
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

function referenceName(id?: string, name?: string) {
  if (!id) return "未指定"
  return name ? `${name} · ${id}` : id
}

function agentLabel(issue: Issue, agents: Array<{ id: string; name: string }>) {
  if (!issue.assigneeAgentId) return "未指定 Agent"
  return (
    agents.find((candidate) => candidate.id === issue.assigneeAgentId)?.name ??
    issue.assigneeAgentId
  )
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  if (size < 1024 * 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`
  }
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`
}
