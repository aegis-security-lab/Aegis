import {
  ListTodo,
} from "lucide-react"
import { Link, useSearchParams } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { SearchInput } from "@/components/ui/search-input"
import { TaskRowMenu } from "@/components/task-row-menu"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Progress } from "@/components/ui/progress"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { formatCost, formatTime } from "@/lib/format"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { useAppState } from "@/lib/state"
import type { Issue, IssueRuntimeView, Task } from "@/types"

type TaskFilter = "all" | "active" | "review" | "done" | "attention"

export function TasksPage() {
  const { state } = useAppState()
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get("q") ?? ""
  const filterParam = searchParams.get("status")
  const filter: TaskFilter = isTaskFilter(filterParam) ? filterParam : "all"
  const issues = state?.issues ?? []
  const executions = state?.executions ?? []
  const tasks = state?.tasks ?? []
  const runtimeMap = issueRuntimeMap(state?.issueRuntimes ?? [])

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
    .map((task) => taskRow(task, issues, executions, runtimeMap))
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
        return (
          row.runtime!.state !== "in_review" &&
          ["running", "waiting", "pending"].includes(row.runtime!.kind)
        )
      }
      if (filter === "review") return row.runtime!.state === "in_review"
      if (filter === "done") return row.runtime!.kind === "completed"
      return ["failed", "cancelled"].includes(row.runtime!.kind)
    })
    .sort((left, right) =>
      right.task.updatedAt.localeCompare(left.task.updatedAt)
    )

  return (
    <div className="flex w-full flex-col gap-5">
      <PageHeader
        eyebrow="Work"
        title="任务"
        description="按状态定位目标，进入任务后查看计划树、执行记录和交付文件。"
      />

      <Card className="min-w-0">
        <CardHeader className="flex flex-col gap-3 border-b pb-3 lg:flex-row lg:items-center">
          <SearchInput
            value={query}
            onChange={setQuery}
            ariaLabel="搜索任务"
            placeholder="搜索标题或编号…"
          />
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
                      <IssueRuntimeBadge runtime={row.runtime!} />
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

                  <TaskRowMenu task={row.task} />
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
  executions: Array<{ issueId: string; status: string; cost: number }>,
  runtimeMap: Map<string, IssueRuntimeView>
) {
  const runs = issues
    .filter((issue) => !issue.parentId && issue.taskSourceId === task.id)
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt))
  const latest = runs[0]
  const runtime = latest
    ? issueRuntimeOrUnavailable(runtimeMap, latest.id)
    : undefined
  const issueIDs = latest
    ? taskTreeIssueIDs(latest.id, issues)
    : new Set<string>()
  const descendants = Math.max(0, issueIDs.size - 1)
  const completed = issues.filter(
    (issue) =>
      issue.id !== latest?.id &&
      issueIDs.has(issue.id) &&
      ["done", "failed", "budget_exceeded", "cancelled"].includes(issue.status)
  ).length
  const progress = descendants
    ? Math.round((completed / descendants) * 100)
    : runtime?.kind === "completed"
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
  return {
    task,
    runs,
    latest,
    runtime,
    progress,
    active,
    cost,
    issueCount: issueIDs.size,
  }
}

function isTaskFilter(value: string | null): value is TaskFilter {
  return ["all", "active", "review", "done", "attention"].includes(value ?? "")
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
