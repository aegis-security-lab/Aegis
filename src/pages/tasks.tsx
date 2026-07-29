import * as React from "react"
import {
  Copy,
  ListTodo,
  MoreHorizontal,
  Plus,
  RotateCcw,
  Search,
} from "lucide-react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { restartTask } from "@/lib/api"
import { formatCost, formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { Issue, Task } from "@/types"

type TaskFilter = "all" | "active" | "review" | "done" | "attention"

export function TasksPage() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [restarting, setRestarting] = React.useState<string | null>(null)
  const query = searchParams.get("q") ?? ""
  const filterParam = searchParams.get("status")
  const filter: TaskFilter = isTaskFilter(filterParam) ? filterParam : "all"
  const issues = state?.issues ?? []
  const executions = state?.executions ?? []
  const tasks = state?.tasks ?? []

  const setQuery = (value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value) next.set("q", value)
    else next.delete("q")
    setSearchParams(next, { replace: true })
  }

  const setFilter = (value: TaskFilter) => {
    const next = new URLSearchParams(searchParams)
    if (value === "all") next.delete("status")
    else next.set("status", value)
    setSearchParams(next, { replace: true })
  }

  const rows = tasks
    .map((task) => taskRow(task, issues, executions))
    .filter((row) => {
      const normalized = query.trim().toLocaleLowerCase()
      const matchesQuery =
        !normalized ||
        row.task.title.toLocaleLowerCase().includes(normalized) ||
        row.latest?.identifier.toLocaleLowerCase().includes(normalized)
      if (!matchesQuery) return false
      if (filter === "all") return true
      if (!row.latest) return filter === "attention"
      if (filter === "active") {
        return ["backlog", "todo", "in_progress"].includes(row.latest.status)
      }
      if (filter === "review") return row.latest.status === "in_review"
      if (filter === "done") return row.latest.status === "done"
      return ["blocked", "failed", "cancelled"].includes(row.latest.status)
    })
    .sort((left, right) =>
      right.task.updatedAt.localeCompare(left.task.updatedAt)
    )

  const cloneTask = (task: Task) => {
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
          containerProfileId: state?.containerProfiles.some(
            (profile) =>
              profile.id === task.containerProfileId && profile.enabled
          )
            ? task.containerProfileId
            : undefined,
          context: task.context,
          constraints: task.constraints,
          timeBudgetMinutes: task.timeBudgetMinutes,
          humanValidationFallback: task.humanValidationFallback,
        },
      },
    })
  }

  const restart = async (task: Task, runs: Issue[]) => {
    setRestarting(task.id)
    try {
      const next = await restartTask(task.id)
      toast.success("已创建新的任务执行", {
        description: `${next.identifier} · 第 ${runs.length + 1} 次执行`,
      })
      navigate(`/tasks/${next.id}`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重新启动失败")
    } finally {
      setRestarting(null)
    }
  }

  return (
    <div className="flex w-full flex-col gap-5">
      <PageHeader
        eyebrow="Work"
        title="任务"
        description="按状态定位目标，进入任务后查看计划树、执行记录和交付文件。"
        actions={
          <Button
            size="sm"
            render={<Link to="/tasks/new" />}
            nativeButton={false}
          >
            <Plus data-icon="inline-start" />
            新建任务
          </Button>
        }
      />

      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 border-b pb-3 lg:flex-row lg:items-center">
          <div className="relative min-w-0 flex-1 lg:max-w-sm">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label="搜索任务"
              placeholder="搜索标题或编号…"
              className="pl-8"
            />
          </div>
          <ToggleGroup
            value={[filter]}
            onValueChange={(value) =>
              setFilter((value[0] as TaskFilter | undefined) ?? "all")
            }
            variant="outline"
            size="sm"
            aria-label="按状态筛选任务"
            className="w-full justify-start overflow-x-auto lg:w-auto"
          >
            <ToggleGroupItem value="all">全部</ToggleGroupItem>
            <ToggleGroupItem value="active">进行中</ToggleGroupItem>
            <ToggleGroupItem value="review">待复核</ToggleGroupItem>
            <ToggleGroupItem value="attention">异常</ToggleGroupItem>
            <ToggleGroupItem value="done">已完成</ToggleGroupItem>
          </ToggleGroup>
        </CardHeader>

        <CardContent className="px-0">
          {tasks.length === 0 ? (
            <Empty className="border-0 py-14">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ListTodo />
                </EmptyMedia>
                <EmptyTitle>还没有任务</EmptyTitle>
                <EmptyDescription>
                  发布目标后，Aegis 会创建计划并启动真实执行。
                </EmptyDescription>
              </EmptyHeader>
              <Button
                size="sm"
                render={<Link to="/tasks/new" />}
                nativeButton={false}
              >
                <Plus data-icon="inline-start" />
                发布第一个任务
              </Button>
            </Empty>
          ) : rows.length === 0 ? (
            <Empty className="border-0 py-14">
              <EmptyHeader>
                <EmptyTitle>没有匹配的任务</EmptyTitle>
                <EmptyDescription>清除搜索词或切换状态筛选。</EmptyDescription>
              </EmptyHeader>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setSearchParams({}, { replace: true })
                }}
              >
                清除筛选
              </Button>
            </Empty>
          ) : (
            <div className="divide-y divide-border/70">
              <div className="hidden grid-cols-[6rem_minmax(220px,1fr)_9rem_5rem_6rem_8rem_2.5rem] items-center gap-3 px-4 py-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase xl:grid">
                <span>状态</span>
                <span>任务</span>
                <span>进度</span>
                <span>活跃</span>
                <span>成本</span>
                <span>更新</span>
                <span className="sr-only">操作</span>
              </div>
              {rows.map((row) => (
                <div
                  key={row.task.id}
                  className="group relative grid min-w-0 gap-3 px-4 py-3 pr-12 transition-colors duration-150 hover:bg-muted/35 xl:grid-cols-[6rem_minmax(220px,1fr)_9rem_5rem_6rem_8rem_2.5rem] xl:items-center xl:pr-4"
                >
                  <div className="flex items-center gap-2 xl:block">
                    {row.latest ? (
                      <StatusBadge status={row.latest.status} />
                    ) : (
                      <Badge variant="outline">待初始化</Badge>
                    )}
                    <span className="font-mono text-[11px] text-muted-foreground xl:hidden">
                      {row.latest?.identifier ?? "—"}
                    </span>
                  </div>

                  <div className="min-w-0">
                    {row.latest ? (
                      <Link
                        to={`/tasks/${row.latest.id}`}
                        className="flex min-h-11 items-center truncate text-sm font-medium hover:text-primary focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none xl:block xl:min-h-0"
                      >
                        {row.task.title}
                      </Link>
                    ) : (
                      <p className="truncate text-sm font-medium">
                        {row.task.title}
                      </p>
                    )}
                    <p className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
                      <span className="hidden font-mono xl:inline">
                        {row.latest?.identifier ?? "尚无执行"}
                      </span>
                      <span className="truncate">{row.task.workspace}</span>
                      <span>第 {row.runs.length} 次执行</span>
                    </p>
                  </div>

                  <div className="min-w-0">
                    <div className="mb-1 flex items-center justify-between text-[11px] text-muted-foreground">
                      <span className="xl:hidden">完成度</span>
                      <span className="tabular-nums">{row.progress}%</span>
                    </div>
                    <Progress
                      value={row.progress}
                      aria-label={`${row.task.title} 完成度 ${row.progress}%`}
                    />
                  </div>

                  <TaskMeta label="活跃执行" value={row.active} />
                  <TaskMeta label="成本" value={formatCost(row.cost)} />
                  <TaskMeta
                    label="更新"
                    value={formatTime(row.task.updatedAt)}
                  />

                  <div className="absolute top-3 right-3 xl:static">
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`任务操作：${row.task.title}`}
                          />
                        }
                      >
                        {restarting === row.task.id ? (
                          <Spinner />
                        ) : (
                          <MoreHorizontal />
                        )}
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-40">
                        <DropdownMenuGroup>
                          <DropdownMenuItem onClick={() => cloneTask(row.task)}>
                            <Copy />
                            复制为新任务
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            disabled={restarting === row.task.id}
                            onClick={() => void restart(row.task, row.runs)}
                          >
                            <RotateCcw />
                            重新启动
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function taskRow(
  task: Task,
  issues: Issue[],
  executions: Array<{ issueId: string; status: string; cost: number }>
) {
  const runs = issues
    .filter((issue) => !issue.parentId && issue.taskSourceId === task.id)
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt))
  const latest = runs[0]
  const issueIDs = latest
    ? taskTreeIssueIDs(latest.id, issues)
    : new Set<string>()
  const descendants = Math.max(0, issueIDs.size - 1)
  const completed = issues.filter(
    (issue) =>
      issue.id !== latest?.id &&
      issueIDs.has(issue.id) &&
      ["done", "cancelled"].includes(issue.status)
  ).length
  const progress = descendants
    ? Math.round((completed / descendants) * 100)
    : latest?.status === "done"
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
  return { task, runs, latest, progress, active, cost }
}

function isTaskFilter(value: string | null): value is TaskFilter {
  return ["all", "active", "review", "done", "attention"].includes(
    value ?? ""
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

function TaskMeta({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex items-center justify-between gap-3 text-xs xl:block">
      <span className="text-muted-foreground xl:sr-only">{label}</span>
      <span className="truncate tabular-nums">{value}</span>
    </div>
  )
}
