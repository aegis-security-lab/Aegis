import * as React from "react"
import { ArrowRight, Copy, ListTodo, RotateCcw } from "lucide-react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
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
import { Progress } from "@/components/ui/progress"
import { formatCost, formatTime } from "@/lib/format"
import { restartTask } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { Issue } from "@/types"

export function TasksPage() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const [restarting, setRestarting] = React.useState<string | null>(null)
  const issues = state?.issues ?? []
  const executions = state?.executions ?? []
  const tasks = state?.tasks ?? []

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow="Workspace"
        title="任务"
        description="顶层 Issue 代表用户目标；进度由它的子 Issues 自动汇总。"
      />
      {tasks.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ListTodo />
                </EmptyMedia>
                <EmptyTitle>还没有任务</EmptyTitle>
                <EmptyDescription>
                  发布目标后，Pi 会创建真实的执行计划。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {tasks.map((task) => {
            const runs = issues
              .filter(
                (issue) => !issue.parentId && issue.taskSourceId === task.id
              )
              .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
            const latest = runs[0]
            if (!latest) return null
            const issueIDs = taskTreeIssueIDs(latest.id, issues)
            const descendants = issueIDs.size - 1
            const completed = issues.filter(
              (issue) =>
                issue.id !== latest.id &&
                issueIDs.has(issue.id) &&
                ["done", "cancelled"].includes(issue.status)
            ).length
            const abandoned = issues.filter(
              (issue) => issueIDs.has(issue.id) && issue.objectiveAbandoned
            ).length
            const progress = descendants
              ? Math.round((completed / descendants) * 100)
              : latest.status === "done"
                ? 100
                : 0
            const treeExecutions = executions.filter((execution) =>
              issueIDs.has(execution.issueId)
            )
            const active = treeExecutions.filter((execution) =>
              ["queued", "starting", "running", "waiting_approval"].includes(
                execution.status
              )
            ).length
            const cost = treeExecutions.reduce(
              (total, execution) => total + execution.cost,
              0
            )

            return (
              <Card
                key={task.id}
                className="group transition-shadow hover:shadow-md"
              >
                <CardHeader>
                  <div className="flex min-w-0 flex-col gap-3">
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-xs text-muted-foreground">
                        {latest.identifier}
                      </span>
                      <StatusBadge status={latest.status} />
                      <Badge variant="outline">执行 {runs.length} 次</Badge>
                      {abandoned > 0 && (
                        <Badge variant="outline">放弃目标 {abandoned}</Badge>
                      )}
                    </div>
                    <CardTitle
                      className="line-clamp-2 leading-6 break-all"
                      title={task.title}
                    >
                      {task.title}
                    </CardTitle>
                  </div>
                  <CardAction>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      render={
                        <Link
                          to={`/tasks/${latest.id}`}
                          aria-label="查看任务"
                        />
                      }
                      nativeButton={false}
                    >
                      <ArrowRight />
                    </Button>
                  </CardAction>
                </CardHeader>
                <CardContent className="flex flex-col gap-5">
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <Stat label="子 Issues" value={descendants} />
                    <Stat label="活跃执行" value={active} />
                    <Stat label="完成度" value={`${progress}%`} />
                    <Stat label="消耗金额" value={formatCost(cost)} />
                  </div>
                  <Progress value={progress} className="h-1.5" />
                </CardContent>
                <CardFooter>
                  <div className="flex w-full items-center justify-between gap-3">
                    <p className="min-w-0 truncate text-xs text-muted-foreground">
                      {task.workspace} · {formatTime(task.updatedAt)}
                    </p>
                    <div className="flex shrink-0 items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          navigate("/tasks/new", {
                            state: {
                              clone: {
                                projectId: task.projectId,
                                title: task.title,
                                description: task.description,
                                objective: task.objective,
                                priority: task.priority,
                                workMode: task.workMode,
                                assigneeAgentId: task.assigneeAgentId,
                                containerProfileId:
                                  state?.containerProfiles.some(
                                    (profile) =>
                                      profile.id === task.containerProfileId &&
                                      profile.enabled
                                  )
                                    ? task.containerProfileId
                                    : undefined,
                                context: task.context,
                                constraints: task.constraints,
                                timeBudgetMinutes: task.timeBudgetMinutes,
                                humanValidationFallback:
                                  task.humanValidationFallback,
                              },
                            },
                          })
                        }
                      >
                        <Copy data-icon="inline-start" />
                        复制
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={restarting === task.id}
                        onClick={async () => {
                          setRestarting(task.id)
                          try {
                            const next = await restartTask(task.id)
                            toast.success("已创建新的任务执行", {
                              description: `${next.identifier} · 第 ${runs.length + 1} 次执行`,
                            })
                            navigate(`/tasks/${next.id}`)
                          } catch (error) {
                            toast.error(
                              error instanceof Error
                                ? error.message
                                : "重新启动失败"
                            )
                          } finally {
                            setRestarting(null)
                          }
                        }}
                      >
                        <RotateCcw data-icon="inline-start" />
                        重新启动
                      </Button>
                    </div>
                  </div>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}

function taskTreeIssueIDs(rootID: string, issues: Issue[]) {
  const ids = new Set([rootID])
  let changed = true
  while (changed) {
    changed = false
    for (const issue of issues) {
      if (issue.parentId && ids.has(issue.parentId) && !ids.has(issue.id)) {
        ids.add(issue.id)
        changed = true
      }
    }
  }
  return ids
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg bg-muted/50 p-3">
      <p className="text-muted-foreground">{label}</p>
      <p className="mt-1 text-sm font-medium tabular-nums">{value}</p>
    </div>
  )
}
