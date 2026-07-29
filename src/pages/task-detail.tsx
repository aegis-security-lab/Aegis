import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import { Clock3, Copy, File, Folder, LinkIcon, Save } from "lucide-react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { StatusBadge } from "@/components/status-badge"
import { PageHeader } from "@/components/page-header"
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
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { fetchIssue, fetchTaskWorkspace, updateTaskBudget } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, IssueDetail, Task, TaskWorkspace } from "@/types"

export function TaskDetailPage() {
  const { issueId } = useParams()
  const navigate = useNavigate()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [workspace, setWorkspace] = React.useState<TaskWorkspace | null>(null)
  const [workspaceError, setWorkspaceError] = React.useState("")
  const [loading, setLoading] = React.useState(true)
  const [now, setNow] = React.useState(() => Date.now())
  const [budgetInput, setBudgetInput] = React.useState("")
  const [savingBudget, setSavingBudget] = React.useState(false)

  React.useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 10000)
    return () => window.clearInterval(timer)
  }, [])

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
        const minutes = issueResult.value.issue.timeBudgetMinutes
        setBudgetInput(minutes ? String(minutes) : "")
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
  const budgetMinutes = issue.timeBudgetMinutes ?? source?.timeBudgetMinutes ?? null
  const startedAt = issue.startedAt ? Date.parse(issue.startedAt) : Date.parse(issue.createdAt)
  const elapsedMinutes = Math.max(0, (now - startedAt) / 60000)
  const progress = budgetMinutes ? Math.min(100, (elapsedMinutes / budgetMinutes) * 100) : 0
  const remainingMinutes = budgetMinutes ? Math.max(0, budgetMinutes - elapsedMinutes) : null
  const saveBudget = async () => {
    const minutes = Number.parseInt(budgetInput, 10)
    if (!Number.isFinite(minutes) || minutes <= 0) { toast.error("请输入大于 0 的分钟数"); return }
    setSavingBudget(true)
    try {
      await updateTaskBudget(issue.taskSourceId || issue.id, minutes)
      setDetail((current) => current ? { ...current, issue: { ...current.issue, timeBudgetMinutes: minutes } } : current)
      toast.success("任务时间预算已更新")
    } catch (error) { toast.error(error instanceof Error ? error.message : "更新时间预算失败") }
    finally { setSavingBudget(false) }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={`任务 / ${issue.identifier}`}
        title={parameters.title}
        description={parameters.objective || "查看执行进度、计划与运行记录。"}
        actions={
          <>
            <StatusBadge status={issue.status} />
            <Button
              variant="outline"
              size="sm"
              render={<Link to={`/issues/${issue.id}`} />}
              nativeButton={false}
            >
              <LinkIcon data-icon="inline-start" />
              查看计划树
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => navigate("/tasks/new", { state: { clone: parameters } })}
            >
              <Copy data-icon="inline-start" />
              复制任务
            </Button>
          </>
        }
      />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Clock3 className="size-4" />任务时间进度</CardTitle>
          <CardDescription>根 Issue 的本次执行预算；子 Issue 不单独消耗任务预算。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
            <span>{budgetMinutes ? `已用 ${formatDuration(elapsedMinutes)} / ${budgetMinutes} 分钟` : "未设置时间预算"}</span>
            <span className={remainingMinutes !== null && remainingMinutes <= 0 ? "font-medium text-destructive" : "text-muted-foreground"}>{remainingMinutes === null ? "无限制" : remainingMinutes <= 0 ? "已超出预算" : `剩余 ${formatDuration(remainingMinutes)}`}</span>
          </div>
          {budgetMinutes ? <div className="h-2 overflow-hidden rounded-full bg-muted"><div className={cn("h-full w-full origin-left rounded-full transition-[transform,background-color] duration-200", progress >= 100 ? "bg-destructive" : progress >= 80 ? "bg-warning" : "bg-primary")} style={{ transform: `scaleX(${progress / 100})` }} /></div> : null}
          <div className="flex max-w-sm items-center gap-2">
            <Input type="number" min={1} value={budgetInput} onChange={(event) => setBudgetInput(event.target.value)} placeholder="分钟" />
            <Button variant="outline" onClick={() => void saveBudget()} disabled={savingBudget}>{savingBudget ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}调整预算</Button>
          </div>
        </CardContent>
      </Card>

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
          <TaskParameter
            label={parameters.containerProfileId ? "容器工作目录" : "工作目录"}
            value={
              parameters.containerProfileId
                ? state?.containers.find(
                    (container) => container.id === parameters.containerId
                  )?.workspacePath ??
                  state?.containerProfiles.find(
                    (profile) => profile.id === parameters.containerProfileId
                  )?.workspacePath ??
                  "容器尚未创建"
                : parameters.workspace
            }
            mono
          />
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
  if (!issue.assigneeAgentId) return "未指定负责人"
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

function formatDuration(minutes: number) {
  if (minutes < 1) return `${Math.round(minutes * 60)} 秒`
  if (minutes < 60) return `${Math.floor(minutes)} 分钟`
  return `${Math.floor(minutes / 60)} 小时 ${Math.floor(minutes % 60)} 分钟`
}
