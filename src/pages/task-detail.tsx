import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  Activity,
  Download,
  LinkIcon,
  MessageSquare,
  Paperclip,
  Radio,
  Smartphone,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"

import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { Badge } from "@/components/ui/badge"
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
import {
  fetchIssue,
  fetchTaskAgents,
  fetchTaskPhones,
} from "@/lib/api"
import { formatBytes, formatTime } from "@/lib/format"
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
} from "@/types"

export function TaskDetailPage() {
  const { issueId } = useParams()
  const { state } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [phones, setPhones] = React.useState<TaskPhone[]>([])
  const [taskAgents, setTaskAgents] = React.useState<TaskAgent[]>([])
  const [phoneError, setPhoneError] = React.useState("")
  const [loading, setLoading] = React.useState(true)

  React.useEffect(() => {
    if (!issueId) return
    let active = true
    setLoading(true)
    void Promise.allSettled([
      fetchIssue(issueId),
      fetchTaskPhones(issueId),
      fetchTaskAgents(issueId),
    ]).then(([issueResult, phoneResult, taskAgentResult]) => {
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
  const source = state?.tasks.find(
    (candidate) => candidate.id === issue.taskSourceId
  )
  const parameters = source ?? issue
  const runs = taskRuns(issue, source, state?.issues ?? [issue])

  return (
    <div className="relative size-full min-h-0 overflow-hidden">
      <div className="size-full min-h-0 overflow-y-auto pr-1 lg:pr-[380px]">
        <div className="flex min-h-full flex-col gap-5">
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
