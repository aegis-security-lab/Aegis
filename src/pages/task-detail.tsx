import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  Activity,
  CircleStop,
  Clock3,
  Copy,
  HardDrive,
  LinkIcon,
  MessageSquare,
  Radio,
  Save,
  Smartphone,
} from "lucide-react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { CancelTaskDialog } from "@/components/cancel-task-dialog"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
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
import { Spinner } from "@/components/ui/spinner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  fetchIssue,
  fetchTaskAgents,
  fetchTaskPhones,
  fetchTaskWorkspace,
  updateTaskBudget,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  AgentDefinition,
  Issue,
  IssueDetail,
  IssueRuntimeView,
  Task,
  TaskAgent,
  TaskPhone,
  TaskWorkspace,
} from "@/types"

export function TaskDetailPage() {
  const { issueId } = useParams()
  const navigate = useNavigate()
  const { state, refresh } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [workspace, setWorkspace] = React.useState<TaskWorkspace | null>(null)
  const [workspaceError, setWorkspaceError] = React.useState("")
  const [phones, setPhones] = React.useState<TaskPhone[]>([])
  const [taskAgents, setTaskAgents] = React.useState<TaskAgent[]>([])
  const [phoneError, setPhoneError] = React.useState("")
  const [loading, setLoading] = React.useState(true)
  const [now, setNow] = React.useState(() => Date.now())
  const [budgetInput, setBudgetInput] = React.useState("")
  const [savingBudget, setSavingBudget] = React.useState(false)
  const [cancelOpen, setCancelOpen] = React.useState(false)

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
      fetchTaskPhones(issueId),
      fetchTaskAgents(issueId),
    ]).then(([issueResult, workspaceResult, phoneResult, taskAgentResult]) => {
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
      if (phoneResult.status === "fulfilled") {
        setPhones(phoneResult.value.phones)
        setPhoneError("")
      } else {
        setPhones([])
        setPhoneError(
          phoneResult.reason instanceof Error
            ? phoneResult.reason.message
            : "读取任务手机失败"
        )
      }
      if (taskAgentResult.status === "fulfilled") {
        setTaskAgents(taskAgentResult.value.agents)
      } else {
        setTaskAgents([])
      }
      setLoading(false)
    })
    return () => {
      active = false
    }
  }, [issueId])

  React.useEffect(() => {
    if (!issueId) return
    let active = true
    const refreshDevices = () => {
      void Promise.allSettled([
        fetchTaskPhones(issueId),
        fetchTaskAgents(issueId),
      ]).then(([phoneResult, agentResult]) => {
        if (!active) return
        if (phoneResult.status === "fulfilled") {
          setPhones(phoneResult.value.phones)
          setPhoneError("")
        }
        if (agentResult.status === "fulfilled") {
          setTaskAgents(agentResult.value.agents)
        }
      })
    }
    const timer = window.setInterval(refreshDevices, 10000)
    return () => {
      active = false
      window.clearInterval(timer)
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
  const runtimeMap = issueRuntimeMap([
    ...(detail.runtime ? [detail.runtime] : []),
    ...(state?.issueRuntimes ?? []),
  ])
  const runtime = issueRuntimeOrUnavailable(runtimeMap, issue.id)
  const source = state?.tasks.find(
    (candidate) => candidate.id === issue.taskSourceId
  )
  const parameters = source ?? issue
  const runs = taskRuns(issue, source, state?.issues ?? [issue])
  const budgetMinutes =
    issue.timeBudgetMinutes ?? source?.timeBudgetMinutes ?? null
  const startedAt = Date.parse(issue.createdAt)
  const elapsedMinutes = Math.max(0, (now - startedAt) / 60000)
  const progress = budgetMinutes
    ? Math.min(100, (elapsedMinutes / budgetMinutes) * 100)
    : 0
  const remainingMinutes = budgetMinutes
    ? Math.max(0, budgetMinutes - elapsedMinutes)
    : null
  const taskIssues = taskTreeIssues(issue, state?.issues ?? [issue])
  const taskIssueIDs = new Set(taskIssues.map((candidate) => candidate.id))
  const activeExecutions = (state?.executions ?? []).filter(
    (execution) =>
      taskIssueIDs.has(execution.issueId) &&
      ["queued", "starting", "running", "waiting_approval"].includes(
        execution.status
      )
  ).length
  const saveBudget = async () => {
    const minutes = Number.parseInt(budgetInput, 10)
    if (!Number.isFinite(minutes) || minutes <= 0) {
      toast.error("请输入大于 0 的分钟数")
      return
    }
    setSavingBudget(true)
    try {
      await updateTaskBudget(issue.taskSourceId || issue.id, minutes)
      setDetail((current) =>
        current
          ? {
              ...current,
              issue: { ...current.issue, timeBudgetMinutes: minutes },
            }
          : current
      )
      toast.success("任务时间预算已更新")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新时间预算失败")
    } finally {
      setSavingBudget(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={`任务 / ${issue.identifier}`}
        title={parameters.title}
        description={parameters.objective || "查看执行进度、计划与运行记录。"}
        actions={
          <>
            <IssueRuntimeBadge runtime={runtime} />
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
              onClick={() =>
                navigate("/tasks/new", { state: { clone: parameters } })
              }
            >
              <Copy data-icon="inline-start" />
              复制任务
            </Button>
            {!isTerminalTask(issue.status) ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setCancelOpen(true)}
              >
                <CircleStop data-icon="inline-start" />
                取消任务
              </Button>
            ) : null}
          </>
        }
      />
      <CancelTaskDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        rootIssueId={issue.id}
        taskTitle={parameters.title}
        totalIssues={taskIssues.length}
        activeExecutions={activeExecutions}
        onCancelled={async (result) => {
          setDetail((current) =>
            current ? { ...current, issue: result.task } : current
          )
          await refresh()
        }}
      />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Clock3 className="size-4" />
            任务时间进度
          </CardTitle>
          <CardDescription>
            整个 Task
            从创建时开始计算的总时钟墙预算；等待、休眠和重新执行都不会重置。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
            <span>
              {budgetMinutes
                ? `已用 ${formatDuration(elapsedMinutes)} / ${budgetMinutes} 分钟`
                : "未设置时间预算"}
            </span>
            <span
              className={
                remainingMinutes !== null && remainingMinutes <= 0
                  ? "font-medium text-destructive"
                  : "text-muted-foreground"
              }
            >
              {remainingMinutes === null
                ? "无限制"
                : remainingMinutes <= 0
                  ? "已超出预算"
                  : `剩余 ${formatDuration(remainingMinutes)}`}
            </span>
          </div>
          {budgetMinutes ? (
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <div
                className={cn(
                  "h-full w-full origin-left rounded-full transition-[transform,background-color] duration-200",
                  progress >= 100
                    ? "bg-destructive"
                    : progress >= 80
                      ? "bg-warning"
                      : "bg-primary"
                )}
                style={{ transform: `scaleX(${progress / 100})` }}
              />
            </div>
          ) : null}
          <div className="flex max-w-sm items-center gap-2">
            <Input
              type="number"
              min={1}
              value={budgetInput}
              onChange={(event) => setBudgetInput(event.target.value)}
              placeholder="分钟"
            />
            <Button
              variant="outline"
              onClick={() => void saveBudget()}
              disabled={savingBudget}
            >
              {savingBudget ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Save data-icon="inline-start" />
              )}
              调整预算
            </Button>
          </div>
        </CardContent>
      </Card>

      <TaskPhoneBelt
        root={issue}
        issues={state?.issues ?? [issue]}
        agents={state?.agents ?? []}
        taskAgents={taskAgents}
        phones={phones}
        runtimes={state?.issueRuntimes ?? []}
        error={phoneError}
      />

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
                ? (state?.containers.find(
                    (container) => container.id === parameters.containerId
                  )?.workspacePath ??
                  state?.containerProfiles.find(
                    (profile) => profile.id === parameters.containerProfileId
                  )?.workspacePath ??
                  "容器尚未创建")
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
          <TaskParameter label="协作模式" value="Board Autonomy" />
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
              <IssueRuntimeBadge
                runtime={issueRuntimeOrUnavailable(runtimeMap, run.id)}
              />
              <LinkIcon className="size-4 shrink-0 text-muted-foreground" />
            </Link>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>任务工作区</CardTitle>
          <CardDescription className="break-all">
            {workspace?.root ?? parameters.workspace}
            {workspace ? " · Docker 私有卷" : ""}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {workspaceError ? (
            <p className="text-sm text-destructive">{workspaceError}</p>
          ) : workspace ? (
            <div className="flex gap-3 rounded-lg border bg-muted/20 p-4">
              <HardDrive className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
              <div className="space-y-1 text-sm">
                <p className="font-medium">文件仅保存在这个任务的容器卷中</p>
                <p className="text-muted-foreground">
                  同一任务的所有 Agent
                  共享该工作区，不映射或复制到宿主机。需要交付给你的文件会作为
                  Issue 评论附件发布，可在对应执行记录中查看和下载。
                </p>
              </div>
            </div>
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

function TaskPhoneBelt({
  root,
  issues,
  agents,
  taskAgents,
  phones,
  runtimes,
  error,
}: {
  root: Issue
  issues: Issue[]
  agents: AgentDefinition[]
  taskAgents: TaskAgent[]
  phones: TaskPhone[]
  runtimes: IssueRuntimeView[]
  error: string
}) {
  const taskIssues = taskTreeIssues(root, issues)
  const runtimeMap = issueRuntimeMap(runtimes)
  const issueByIdentity = new Map(
    taskIssues
      .filter((issue) => issue.assigneeTaskAgentId)
      .map((issue) => [issue.assigneeTaskAgentId!, issue])
  )
  const phoneByIdentity = new Map(
    phones
      .filter((phone) => phone.taskAgentId)
      .map((phone) => [phone.taskAgentId!, phone])
  )
  const visibleAgents = taskAgents.filter(
    (identity) =>
      issueByIdentity.has(identity.id) || phoneByIdentity.has(identity.id)
  )
  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b bg-muted/20">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Smartphone className="size-4" />
              任务小队设备
            </CardTitle>
            <CardDescription className="mt-1">
              每个任务成员实例拥有独立会话与独立手机；页面栈、草稿、Relay
              收件箱和操作审计写入 SQLite，并由 taskAgentId 隔离。
            </CardDescription>
          </div>
          <Badge variant="outline" className="gap-1.5 tabular-nums">
            <span className="size-1.5 rounded-full bg-success" />
            {phones.length}/{visibleAgents.length} 已创建
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {error ? (
          <p className="px-5 py-4 text-sm text-destructive">{error}</p>
        ) : null}
        {visibleAgents.length === 0 ? (
          <div className="flex min-h-36 flex-col items-center justify-center px-6 text-center">
            <Smartphone className="mb-3 size-5 text-muted-foreground" />
            <p className="text-sm font-medium">任务编组尚未形成</p>
            <p className="mt-1 text-xs text-muted-foreground">
              Issue 分配给 Agent 类型时，会自动领取临时名并配置一部任务手机。
            </p>
          </div>
        ) : (
          <div className="grid sm:grid-cols-2 xl:grid-cols-3">
            {visibleAgents.map((identity) => {
              const phone = phoneByIdentity.get(identity.id)
              const issue = issueByIdentity.get(identity.id)
              const agent = agents.find(
                (candidate) => candidate.id === identity.agentId
              )
              return (
                <div
                  key={identity.id}
                  className="min-w-0 border-b p-5 sm:border-r xl:[&:nth-child(3n)]:border-r-0"
                >
                  <div className="flex items-start gap-3">
                    <div className="relative flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 font-semibold text-primary">
                      {identity.name.slice(0, 1)}
                      <span
                        className={cn(
                          "absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-card",
                          phone ? "bg-success" : "bg-muted-foreground/35"
                        )}
                      />
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">
                        {identity.name}{" "}
                        <span className="font-normal text-muted-foreground">
                          · {agent?.name ?? identity.agentId}
                        </span>
                      </p>
                      <p className="truncate font-mono text-[11px] text-muted-foreground">
                        {identity.id}
                      </p>
                    </div>
                    {phone ? (
                      <Radio className="size-4 text-success" />
                    ) : (
                      <Spinner className="size-4 text-muted-foreground" />
                    )}
                  </div>
                  {phone ? (
                    <div className="mt-4 space-y-3">
                      <div className="rounded-md border bg-muted/20 p-3">
                        <div className="flex items-center justify-between gap-2 text-xs">
                          <span className="truncate font-medium">
                            {issue?.identifier ?? "等待 Issue"} ·{" "}
                            {issue?.title ?? "尚未绑定"}
                          </span>
                          {issue ? (
                            <IssueRuntimeBadge
                              runtime={issueRuntimeOrUnavailable(
                                runtimeMap,
                                issue.id
                              )}
                            />
                          ) : null}
                        </div>
                        <div className="mt-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                          <Activity className="size-3" />
                          最近信号{" "}
                          {formatTime(phone.updatedAt || identity.updatedAt)}
                        </div>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {phone.installedApps.map((app) => (
                          <Badge key={app} variant="secondary">
                            {app.replace("aegis.", "")}
                          </Badge>
                        ))}
                      </div>
                      <div className="grid grid-cols-2 gap-3 text-xs">
                        <PhoneDatum
                          label="当前页面"
                          value={phone.currentPageId || "Phone Home"}
                        />
                        <PhoneDatum
                          label="操作审计"
                          value={`${phone.actionCount} 条`}
                        />
                      </div>
                      <p
                        className="truncate font-mono text-[10px] text-muted-foreground"
                        title={phone.id}
                      >
                        {phone.id}
                      </p>
                    </div>
                  ) : (
                    <div className="mt-4 rounded-md border border-dashed px-3 py-4 text-xs leading-relaxed text-muted-foreground">
                      <div className="flex items-center gap-2 font-medium text-foreground">
                        <MessageSquare className="size-3.5" />
                        等待手机启动
                      </div>
                      <p className="mt-1">
                        Coordination 会在首次执行前创建并恢复持久状态。
                      </p>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function PhoneDatum({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-muted-foreground">{label}</p>
      <p className="mt-1 truncate font-medium" title={value}>
        {value}
      </p>
    </div>
  )
}

function taskTreeIssues(root: Issue, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  const belongs = (issue: Issue) => {
    let current: Issue | undefined = issue
    const seen = new Set<string>()
    while (current && !seen.has(current.id)) {
      if (current.id === root.id) return true
      seen.add(current.id)
      current = current.parentId ? byID.get(current.parentId) : undefined
    }
    return false
  }
  return issues.filter(belongs)
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

function formatDuration(minutes: number) {
  if (minutes < 1) return `${Math.round(minutes * 60)} 秒`
  if (minutes < 60) return `${Math.floor(minutes)} 分钟`
  return `${Math.floor(minutes / 60)} 小时 ${Math.floor(minutes % 60)} 分钟`
}

function isTerminalTask(status: Issue["status"]) {
  return ["done", "failed", "budget_exceeded", "cancelled"].includes(status)
}
