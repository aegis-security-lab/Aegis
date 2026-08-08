import * as React from "react"
import { ChevronRight, ChevronsDown, ChevronsUp, Plus } from "lucide-react"
import { Link, useParams } from "react-router-dom"

import { CreateIssueDialog } from "@/components/create-issue-dialog"
import { IssueStatusSelect } from "@/components/issue-status-select"
import { IssueTree } from "@/components/issue-tree"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { SearchInput } from "@/components/ui/search-input"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import {
  hasIssueLabel,
  issueWorkflowStatus,
  workflowStatusLabels,
  workflowStatuses,
} from "@/lib/issue-workflow"
import { issuesForTask } from "@/lib/collections"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, IssueRuntimeView, IssueStatus } from "@/types"

const statuses = workflowStatuses

const statusLabels = workflowStatusLabels

type ViewMode = "tree" | "list"
type IssueGroupKey = "running" | "waiting" | "failed" | "finished" | "todo"

const groupOrder: IssueGroupKey[] = [
  "running",
  "waiting",
  "failed",
  "finished",
  "todo",
]

const groupLabels: Record<IssueGroupKey, string> = {
  running: "执行中",
  waiting: "等待中",
  failed: "失败",
  finished: "已结束",
  todo: "待执行",
}

const groupToStatus: Record<IssueGroupKey, IssueStatus> = {
  running: "in_progress",
  waiting: "todo",
  failed: "todo",
  finished: "done",
  todo: "todo",
}

export function IssuesPage() {
  const { state } = useAppState()
  const { taskId } = useParams()
  const [filter, setFilter] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [expansion, setExpansion] = React.useState<"none" | "roots" | "all">(
    "roots"
  )
  const [expansionVersion, setExpansionVersion] = React.useState(0)
  const [viewMode, setViewMode] = React.useState<ViewMode>("tree")
  const [createOpen, setCreateOpen] = React.useState(false)
  const [createStatus, setCreateStatus] = React.useState<IssueStatus>("todo")
  const [openGroups, setOpenGroups] = React.useState<
    Record<IssueGroupKey, boolean>
  >(
    () =>
      Object.fromEntries(groupOrder.map((key) => [key, true])) as Record<
        IssueGroupKey,
        boolean
      >
  )
  const allIssues = React.useMemo(() => state?.issues ?? [], [state?.issues])
  const deferredQuery = React.useDeferredValue(query)
  const task = React.useMemo(
    () => state?.tasks.find((candidate) => candidate.id === taskId),
    [state?.tasks, taskId]
  )
  const taskSourceId = task?.id
  const scopedIssues = React.useMemo(
    () => (taskSourceId ? issuesForTask(taskSourceId, allIssues) : []),
    [allIssues, taskSourceId]
  )
  const matched = React.useMemo(() => {
    const normalizedQuery = deferredQuery.trim().toLocaleLowerCase()
    return scopedIssues.filter(
      (issue) =>
        (filter === "all" || issueWorkflowStatus(issue.status) === filter) &&
        (!normalizedQuery ||
          `${issue.identifier} ${issue.title}`
            .toLocaleLowerCase()
            .includes(normalizedQuery))
    )
  }, [deferredQuery, filter, scopedIssues])
  const runtimeMap = React.useMemo(
    () => issueRuntimeMap(state?.issueRuntimes ?? []),
    [state?.issueRuntimes]
  )
  const statusCounts = React.useMemo(() => {
    const counts = new Map<string, number>()
    for (const issue of scopedIssues) {
      const status = issueWorkflowStatus(issue.status)
      counts.set(status, (counts.get(status) ?? 0) + 1)
    }
    return counts
  }, [scopedIssues])
  const displayedIssues = React.useMemo(
    () => includeAncestors(matched, scopedIssues),
    [matched, scopedIssues]
  )
  const listGroups = React.useMemo(() => {
    const groups: Record<IssueGroupKey, Issue[]> = {
      running: [],
      waiting: [],
      failed: [],
      finished: [],
      todo: [],
    }
    for (const issue of matched) {
      const runtime = issueRuntimeOrUnavailable(runtimeMap, issue.id)
      groups[classifyIssue(issue, runtime)].push(issue)
    }
    return groups
  }, [matched, runtimeMap])

  return (
    <div className="flex size-full min-h-0 flex-col gap-4">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <SearchInput
          value={query}
          onChange={setQuery}
          ariaLabel="搜索 Issue"
          placeholder="搜索编号或标题…"
        />
        <ToggleGroup
          className="min-w-0 justify-start overflow-x-auto"
          value={[filter]}
          onValueChange={(value) => setFilter(String(value[0] ?? "all"))}
          variant="outline"
          size="sm"
          aria-label="按状态筛选 Issue"
        >
          <ToggleGroupItem value="all">
            全部{" "}
            <span className="text-muted-foreground tabular-nums">
              {scopedIssues.length}
            </span>
          </ToggleGroupItem>
          {statuses.map((status) => (
            <ToggleGroupItem key={status} value={status}>
              {statusLabels[status]}{" "}
              <span className="text-muted-foreground tabular-nums">
                {statusCounts.get(status) ?? 0}
              </span>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        {viewMode === "tree" ? (
          <Button
            size="sm"
            variant="outline"
            className="shrink-0"
            onClick={() => {
              setExpansion(expansion === "all" ? "none" : "all")
              setExpansionVersion((value) => value + 1)
            }}
          >
            {expansion === "all" ? <ChevronsUp /> : <ChevronsDown />}
            <span className="hidden sm:inline">
              {expansion === "all" ? "收起全部" : "展开全部"}
            </span>
          </Button>
        ) : null}
        {taskSourceId ? (
          <Button
            size="sm"
            variant="outline"
            className="shrink-0"
            onClick={() => setCreateOpen(true)}
          >
            <Plus />
            新增
          </Button>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <span className="text-xs text-muted-foreground">树状</span>
          <Switch
            size="sm"
            checked={viewMode === "list"}
            onCheckedChange={(checked) =>
              setViewMode(checked ? "list" : "tree")
            }
            aria-label="切换树状/清单视图"
          />
          <span className="text-xs text-muted-foreground">清单</span>
        </div>
      </div>

      {viewMode === "list" ? (
        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          {groupOrder.map((key) => {
            const items = listGroups[key]
            const open = openGroups[key]
            return (
              <section
                key={key}
                className="border-b border-border/60 last:border-0"
              >
                <div className="group flex h-8 items-center gap-2 rounded-md px-1 hover:bg-muted/40">
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-expanded={open}
                    onClick={() =>
                      setOpenGroups((current) => ({
                        ...current,
                        [key]: !current[key],
                      }))
                    }
                    className="h-full min-w-0 flex-1 justify-start rounded-md px-1 text-xs text-muted-foreground hover:text-foreground"
                  >
                    <ChevronRight
                      className={cn(
                        "size-3.5 transition-transform",
                        open && "rotate-90"
                      )}
                    />
                    {groupLabels[key]}
                    <span className="tabular-nums">{items.length}</span>
                  </Button>
                  {taskSourceId ? (
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      aria-label={`在「${groupLabels[key]}」中新建 Issue`}
                      onClick={() => {
                        setCreateStatus(groupToStatus[key])
                        setCreateOpen(true)
                      }}
                      className="shrink-0 rounded-md text-muted-foreground hover:bg-background hover:text-foreground"
                    >
                      <Plus />
                    </Button>
                  ) : null}
                </div>
                {open ? (
                  <div className="flex flex-col">
                    {items.length === 0 ? (
                      <p className="px-7 py-2 text-xs text-muted-foreground">
                        暂无
                      </p>
                    ) : (
                      items.map((issue) => (
                        <div
                          key={issue.id}
                          className="flex min-h-10 items-center gap-2 rounded-md px-7 py-1 hover:bg-muted/40"
                        >
                          <Link
                            to={`/issues/${issue.id}`}
                            className="flex min-w-0 flex-1 items-center gap-2 text-sm"
                          >
                            <span className="w-18 shrink-0 font-mono text-[11px] text-muted-foreground">
                              {issue.identifier}
                            </span>
                            <span className="min-w-0 flex-1 truncate font-medium">
                              {issue.title}
                            </span>
                          </Link>
                          <IssueStatusSelect issue={issue} align="end" />
                          <IssueRuntimeBadge
                            runtime={issueRuntimeOrUnavailable(
                              runtimeMap,
                              issue.id
                            )}
                          />
                        </div>
                      ))
                    )}
                  </div>
                ) : null}
              </section>
            )
          })}
        </div>
      ) : (
        <div className="min-h-0 flex-1">
          <Card className="flex h-full min-h-0 flex-col overflow-hidden p-0">
            <IssueTree
              key={`${filter}-${expansion}-${expansionVersion}`}
              mode="hierarchy"
              defaultExpansion={expansion}
              issues={displayedIssues}
              relations={state?.relations ?? []}
              agents={state?.agents ?? []}
              taskAgents={state?.taskAgents ?? []}
              runtimes={state?.issueRuntimes ?? []}
              editableStatus
              className="h-full"
            />
          </Card>
        </div>
      )}
      {taskSourceId ? (
        <CreateIssueDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          defaultStatus={createStatus}
          workspace={task?.workspace}
          taskSourceId={taskSourceId}
        />
      ) : null}
    </div>
  )
}

function includeAncestors(matched: Issue[], allIssues: Issue[]) {
  const issueMap = new Map(allIssues.map((issue) => [issue.id, issue]))
  const visible = new Map<string, Issue>()
  for (const issue of matched) {
    visible.set(issue.id, issue)
    let parentID = issue.parentId
    while (parentID) {
      const parent = issueMap.get(parentID)
      if (!parent || visible.has(parent.id)) break
      visible.set(parent.id, parent)
      parentID = parent.parentId
    }
  }
  return [...visible.values()]
}

function classifyIssue(issue: Issue, runtime: IssueRuntimeView): IssueGroupKey {
  if (["completed", "cancelled"].includes(runtime.kind)) return "finished"
  if (runtime.kind === "failed") return "failed"
  if (runtime.kind === "running") return "running"
  if (runtime.kind === "waiting") return "waiting"
  if (["done", "cancelled"].includes(issue.status)) return "finished"
  if (hasIssueLabel(issue, "failed", "budget_exceeded", "blocked")) {
    return "failed"
  }
  return "todo"
}
