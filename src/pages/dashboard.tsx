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
import { Button } from "@/components/ui/button"
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
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { useAppState } from "@/lib/state"

export function DashboardPage() {
  const { state } = useAppState()
  const issues = state?.issues ?? []
  const runtimeMap = issueRuntimeMap(state?.issueRuntimes ?? [])
  const tasks = issues.filter((issue) => !issue.parentId)
  const active = (state?.executions ?? []).filter((execution) =>
    ["queued", "starting", "running", "waiting_approval"].includes(
      execution.status
    )
  )
  const pending = (state?.approvals ?? []).filter(
    (approval) => approval.status === "pending"
  )
  const blocked = issues.filter(
    (issue) => issueRuntimeOrUnavailable(runtimeMap, issue.id).kind === "failed"
  )
  const recent = [...tasks]
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))
    .slice(0, 8)

  return (
    <div className="flex w-full flex-col gap-5">
      <PageHeader
        eyebrow="Control plane"
        title="运行概览"
        description="先处理需要人工介入的事项，再查看任务与执行进度。"
        actions={
          pending.length ? (
            <Button
              size="sm"
              render={<Link to="/approvals" />}
              nativeButton={false}
            >
              <ClipboardCheck data-icon="inline-start" />
              处理 {pending.length} 项审批
            </Button>
          ) : undefined
        }
      />

      <section
        aria-label="运行摘要"
        className="grid divide-y divide-border/70 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4"
      >
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
      </section>

      <div className="grid min-h-0 gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,.65fr)]">
        <Card className="min-w-0">
          <CardHeader className="border-b pb-3">
            <CardTitle>最近任务</CardTitle>
            <CardAction>
              <Button
                variant="ghost"
                size="sm"
                render={<Link to="/tasks" />}
                nativeButton={false}
              >
                查看全部
                <ArrowRight data-icon="inline-end" />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className="px-0">
            {recent.length ? (
              <div className="divide-y divide-border/70">
                {recent.map((issue) => (
                  <Link
                    key={issue.id}
                    to={`/tasks/${issue.id}`}
                    className="grid min-h-14 grid-cols-[5rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 transition-colors duration-150 hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      {issue.identifier}
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate text-sm font-medium">
                        {issue.title}
                      </span>
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {formatTime(issue.updatedAt)}
                      </span>
                    </span>
                    <IssueRuntimeBadge
                      runtime={issueRuntimeOrUnavailable(runtimeMap, issue.id)}
                    />
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
                  const agent = state?.agents.find(
                    (candidate) => candidate.id === execution.agentId
                  )
                  const issue = issues.find(
                    (candidate) => candidate.id === execution.issueId
                  )
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
      className="group relative flex min-h-16 items-center gap-3 px-3 py-2.5 transition-colors duration-150 hover:bg-muted/50 focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset"
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
