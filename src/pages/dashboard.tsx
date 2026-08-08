import * as React from "react"
import {
  Activity,
  ArrowRight,
  Bot,
  CheckSquare2,
  ClipboardCheck,
  ListTodo,
} from "lucide-react"
import { Link } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { StatusBadge } from "@/components/status-badge"
import { buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardAction,
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
import { formatTime } from "@/lib/format"
import { issuesByTask, latestUpdatedAt } from "@/lib/collections"
import {
  issueRuntimeMap,
  issueRuntimeOrUnavailable,
  taskRuntimeView,
} from "@/lib/issue-runtime"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"

const activeExecutionStatuses = new Set([
  "queued",
  "starting",
  "running",
  "waiting_approval",
])

export function DashboardPage() {
  const { state } = useAppState()
  const view = React.useMemo(() => {
    const issues = state?.issues ?? []
    const runtimeMap = issueRuntimeMap(state?.issueRuntimes ?? [])
    const taskIssuesByID = issuesByTask(issues)
    const tasks = state?.tasks ?? []
    const active = (state?.executions ?? []).filter((execution) =>
      activeExecutionStatuses.has(execution.status)
    )
    const pending = (state?.approvals ?? []).filter(
      (approval) => approval.status === "pending"
    )
    const blocked = issues.filter(
      (issue) =>
        issueRuntimeOrUnavailable(runtimeMap, issue.id).kind === "failed"
    )
    return {
      issues,
      runtimeMap,
      tasks,
      active,
      pending,
      blocked,
      recent: tasks
        .map((task) => {
          const taskIssues = taskIssuesByID.get(task.id) ?? []
          return {
            task,
            issueCount: taskIssues.length,
            runtime: taskRuntimeView(task.id, taskIssues, runtimeMap),
            updatedAt: latestUpdatedAt(task.updatedAt, taskIssues),
          }
        })
        .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))
        .slice(0, 8),
      agentById: new Map(
        (state?.agents ?? []).map((agent) => [agent.id, agent])
      ),
      issueById: new Map(issues.map((issue) => [issue.id, issue])),
    }
  }, [state])
  const { runtimeMap, tasks, active, pending, blocked, recent } = view
  const needsAttention = pending.length + blocked.length

  return (
    <div className="flex w-full flex-col gap-5">
      <PageHeader
        eyebrow="Control plane"
        title="运行概览"
        description="实时查看任务、Agent 会话与人工介入状态。"
        actions={
          pending.length ? (
            <Link to="/approvals" className={buttonVariants({ size: "sm" })}>
              <ClipboardCheck data-icon="inline-start" />
              处理 {pending.length} 项审批
            </Link>
          ) : undefined
        }
      />

      <section
        aria-label="运行摘要"
        className="overflow-hidden rounded-xl border border-border/70 bg-muted/25 p-1"
      >
        <div className="flex min-h-11 flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2">
          <div className="flex min-w-0 items-center gap-2.5">
            <span className="relative flex size-2.5" aria-hidden="true">
              <span
                className={cn(
                  "absolute inline-flex size-full animate-ping rounded-full opacity-30",
                  needsAttention ? "bg-warning" : "bg-success"
                )}
              />
              <span
                className={cn(
                  "relative inline-flex size-2.5 rounded-full",
                  needsAttention ? "bg-warning" : "bg-success"
                )}
              />
            </span>
            <div className="min-w-0">
              <p className="text-xs font-medium">
                {needsAttention
                  ? `${needsAttention} 项需要处理`
                  : "运行状态正常"}
              </p>
              <p className="truncate text-[11px] text-muted-foreground">
                实时事件已连接，状态会自动刷新
              </p>
            </div>
          </div>
          <span className="ml-auto font-mono text-[10px] tracking-wide text-muted-foreground uppercase">
            Live runtime rail
          </span>
        </div>
        <div className="grid divide-y divide-border/70 overflow-hidden rounded-lg bg-card shadow-sm ring-1 ring-foreground/5 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4">
          <MetricLink
            icon={ListTodo}
            label="顶层任务"
            value={tasks.length}
            detail="查看任务"
            to="/tasks"
          />
          <MetricLink
            icon={Activity}
            label="活跃执行"
            value={active.length}
            detail="查看会话"
            to="/sessions"
          />
          <MetricLink
            icon={ClipboardCheck}
            label="等待审批"
            value={pending.length}
            detail={pending.length ? "需要处理" : "队列为空"}
            to="/approvals"
            urgent={pending.length > 0}
          />
          <MetricLink
            icon={CheckSquare2}
            label="阻塞或失败"
            value={blocked.length}
            detail={blocked.length ? "需要检查" : "状态正常"}
            to="/issues"
            urgent={blocked.length > 0}
          />
        </div>
      </section>

      <div className="grid min-h-0 gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,.65fr)]">
        <Card className="min-w-0">
          <CardHeader className="border-b pb-3">
            <CardTitle>最近任务</CardTitle>
            <CardAction>
              <Link
                to="/tasks"
                className={buttonVariants({ variant: "ghost", size: "sm" })}
              >
                查看全部
                <ArrowRight data-icon="inline-end" />
              </Link>
            </CardAction>
          </CardHeader>
          <CardContent className="px-0">
            {recent.length ? (
              <div className="divide-y divide-border/70">
                {recent.map(({ task, issueCount, runtime, updatedAt }) => (
                  <Link
                    key={task.id}
                    to={`/tasks/${task.id}`}
                    className="grid min-h-14 grid-cols-[5rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 transition-colors duration-150 hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      {issueCount} Issues
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">
                        {task.title}
                      </span>
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {formatTime(updatedAt)}
                      </span>
                    </span>
                    {runtime ? <IssueRuntimeBadge runtime={runtime} /> : null}
                  </Link>
                ))}
              </div>
            ) : (
              <Empty className="border-0 py-12">
                <EmptyHeader>
                  <EmptyTitle>还没有任务</EmptyTitle>
                  <EmptyDescription>
                    发布一个目标后，这里会显示最新进度。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </CardContent>
        </Card>

        <div className="flex min-w-0 flex-col gap-4">
          {(pending.length > 0 || blocked.length > 0) && (
            <Card>
              <CardHeader className="border-b pb-3">
                <CardTitle>需要处理</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-1 px-2">
                {pending.slice(0, 3).map((approval) => (
                  <Link
                    key={approval.id}
                    to="/approvals"
                    className="flex min-h-12 items-center gap-3 rounded-md px-2 py-2 transition-colors duration-150 hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  >
                    <ClipboardCheck className="size-4 shrink-0 text-warning" />
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {approval.title}
                    </span>
                    <StatusBadge status={approval.status} />
                  </Link>
                ))}
                {blocked.slice(0, 3).map((issue) => (
                  <Link
                    key={issue.id}
                    to={`/issues/${issue.id}`}
                    className="flex min-h-12 items-center gap-3 rounded-md px-2 py-2 transition-colors duration-150 hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  >
                    <CheckSquare2 className="size-4 shrink-0 text-destructive" />
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {issue.identifier} · {issue.title}
                    </span>
                    <IssueRuntimeBadge
                      runtime={issueRuntimeOrUnavailable(runtimeMap, issue.id)}
                    />
                  </Link>
                ))}
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader className="border-b pb-3">
              <CardTitle>运行中的会话</CardTitle>
              <CardAction>
                <span className="text-xs text-muted-foreground tabular-nums">
                  {active.length} 个
                </span>
              </CardAction>
            </CardHeader>
            <CardContent className="flex flex-col gap-1 px-2">
              {active.length ? (
                active.slice(0, 6).map((execution) => {
                  const agent = view.agentById.get(execution.agentId)
                  const issue = view.issueById.get(execution.issueId)
                  return (
                    <Link
                      key={execution.id}
                      to={`/sessions/${execution.id}`}
                      className="flex min-h-12 items-center gap-3 rounded-md px-2 py-2 transition-colors duration-150 hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                    >
                      <Bot className="size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {agent?.name ?? execution.agentId}
                        </span>
                        <span className="block truncate font-mono text-[11px] text-muted-foreground">
                          {execution.currentTool ||
                            issue?.identifier ||
                            execution.model}
                        </span>
                      </span>
                      <StatusBadge status={execution.status} />
                    </Link>
                  )
                })
              ) : (
                <p className="px-2 py-8 text-center text-sm text-muted-foreground">
                  当前没有运行中的会话。
                </p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function MetricLink({
  icon: Icon,
  label,
  value,
  detail,
  to,
  urgent = false,
}: {
  icon: typeof Activity
  label: string
  value: number
  detail: string
  to: string
  urgent?: boolean
}) {
  return (
    <Link
      to={to}
      className="group relative flex min-h-16 items-center gap-3 px-3 py-2.5 transition-[background-color,box-shadow] duration-150 hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
    >
      <Icon
        className={
          urgent ? "size-3.5 text-warning" : "size-3.5 text-muted-foreground"
        }
      />
      <span className="min-w-0 flex-1">
        <span className="block text-[11px] text-muted-foreground">{label}</span>
        <span className="mt-0.5 block text-[11px] text-muted-foreground/80">
          {detail}
        </span>
      </span>
      <span
        className={
          urgent
            ? "text-xl font-semibold text-warning tabular-nums"
            : "text-xl font-semibold tabular-nums"
        }
      >
        {value}
      </span>
    </Link>
  )
}
