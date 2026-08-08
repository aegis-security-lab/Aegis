import * as React from "react"
import { ChevronRight, ChevronsDown, ChevronsUp, Plus } from "lucide-react"
import { useNavigate, useParams } from "react-router-dom"

import { CopyIssueIdentifier } from "@/components/copy-issue-identifier"
import { CreateIssueDialog } from "@/components/create-issue-dialog"
import { IssueStatusSelect } from "@/components/issue-status-select"
import { IssueTree } from "@/components/issue-tree"
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
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, IssueRuntimeView, IssueStatus } from "@/types"

const statuses = workflowStatuses

const statusLabels = workflowStatusLabels

type ViewMode = "tree" | "list"

const ISSUE_VIEW_MODE_STORAGE_KEY = "aegis:issues:view-mode"
const issueFilterItemClassName =
  "aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground aria-pressed:shadow-sm aria-pressed:hover:bg-primary/90 data-[state=on]:border-primary data-[state=on]:bg-primary data-[state=on]:text-primary-foreground data-[state=on]:shadow-sm data-[state=on]:hover:bg-primary/90"

function readIssueViewMode(): ViewMode {
  if (typeof window === "undefined") return "tree"
  try {
    const stored = window.localStorage.getItem(ISSUE_VIEW_MODE_STORAGE_KEY)
    return stored === "list" || stored === "tree" ? stored : "tree"
  } catch {
    return "tree"
  }
}

function writeIssueViewMode(viewMode: ViewMode) {
  try {
    window.localStorage.setItem(ISSUE_VIEW_MODE_STORAGE_KEY, viewMode)
  } catch {
    // Storage can be unavailable in restricted browser contexts. The in-page
    // selection still works for the current visit.
  }
}

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
  const [viewMode, setViewMode] = React.useState<ViewMode>(readIssueViewMode)
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
  const issueById = React.useMemo(
    () => new Map(scopedIssues.map((issue) => [issue.id, issue])),
    [scopedIssues]
  )
  const childrenByParent = React.useMemo(() => {
    const children = new Map<string, Issue[]>()
    for (const issue of scopedIssues) {
      if (!issue.parentId) continue
      const siblings = children.get(issue.parentId) ?? []
      siblings.push(issue)
      children.set(issue.parentId, siblings)
    }
    return children
  }, [scopedIssues])
  const agentNameById = React.useMemo(
    () => new Map((state?.agents ?? []).map((agent) => [agent.id, agent.name])),
    [state?.agents]
  )
  const taskAgentNameById = React.useMemo(
    () =>
      new Map(
        (state?.taskAgents ?? []).map((agent) => [agent.id, agent.name])
      ),
    [state?.taskAgents]
  )

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
          <ToggleGroupItem value="all" className={issueFilterItemClassName}>
            全部{" "}
            <span
              className={cn(
                "tabular-nums",
                filter === "all"
                  ? "text-primary-foreground/80"
                  : "text-muted-foreground"
              )}
            >
              {scopedIssues.length}
            </span>
          </ToggleGroupItem>
          {statuses.map((status) => (
            <ToggleGroupItem
              key={status}
              value={status}
              className={issueFilterItemClassName}
            >
              {statusLabels[status]}{" "}
              <span
                className={cn(
                  "tabular-nums",
                  filter === status
                    ? "text-primary-foreground/80"
                    : "text-muted-foreground"
                )}
              >
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
            onCheckedChange={(checked) => {
              const nextViewMode = checked ? "list" : "tree"
              setViewMode(nextViewMode)
              writeIssueViewMode(nextViewMode)
            }}
            aria-label="切换树状/清单视图"
          />
          <span className="text-xs text-muted-foreground">清单</span>
        </div>
      </div>

      {viewMode === "list" ? (
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto pr-1">
          {groupOrder.map((key) => {
            const items = listGroups[key]
            const open = openGroups[key]
            return (
              <section key={key}>
                <div className="group flex h-10 items-center gap-2 rounded-lg bg-muted/55 px-2">
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
                    className="h-full min-w-0 flex-1 justify-start rounded-md px-0 text-sm font-medium hover:bg-transparent active:not-aria-[haspopup]:scale-100 active:bg-transparent aria-expanded:bg-transparent aria-expanded:text-foreground dark:hover:bg-transparent"
                  >
                    <ChevronRight
                      className={cn(
                        "size-3.5 text-muted-foreground transition-transform",
                        open && "rotate-90"
                      )}
                    />
                    {groupLabels[key]}
                    <span className="font-normal text-muted-foreground tabular-nums">
                      {items.length}
                    </span>
                  </Button>
                  {taskSourceId ? (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`在「${groupLabels[key]}」中新建 Issue`}
                      onClick={() => {
                        setCreateStatus(groupToStatus[key])
                        setCreateOpen(true)
                      }}
                      className="shrink-0 rounded-full text-muted-foreground hover:bg-transparent hover:text-foreground active:not-aria-[haspopup]:scale-100 active:bg-transparent dark:hover:bg-transparent"
                    >
                      <Plus />
                    </Button>
                  ) : null}
                </div>
                {open ? (
                  <div className="divide-y divide-border/50">
                    {items.length === 0 ? (
                      <p className="px-10 py-3 text-xs text-muted-foreground">
                        暂无
                      </p>
                    ) : (
                      items.map((issue) => (
                        <IssueListRow
                          key={issue.id}
                          issue={issue}
                          parent={
                            issue.parentId
                              ? issueById.get(issue.parentId)
                              : undefined
                          }
                          children={childrenByParent.get(issue.id) ?? []}
                          assignee={
                            taskAgentNameById.get(
                              issue.assigneeTaskAgentId ?? ""
                            ) ??
                            agentNameById.get(issue.assigneeAgentId ?? "")
                          }
                        />
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

function IssueListRow({
  issue,
  parent,
  children,
  assignee,
}: {
  issue: Issue
  parent?: Issue
  children: Issue[]
  assignee?: string
}) {
  const navigate = useNavigate()
  const completedChildren = children.filter(
    (child) => issueWorkflowStatus(child.status) === "done"
  ).length

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={`打开 ${issue.identifier} ${issue.title}`}
      onClick={() => navigate(`/issues/${issue.id}`)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          navigate(`/issues/${issue.id}`)
        }
      }}
      className="group/issue-row flex min-h-11 min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 outline-none transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60"
    >
      <CopyIssueIdentifier identifier={issue.identifier} className="hidden w-18 justify-start sm:inline-flex" />
      <IssueStatusSelect
        issue={issue}
        align="start"
        showDot={false}
        ariaLabel={`修改 ${issue.identifier} 的状态`}
        className="shrink-0 px-1.5 text-xs"
      />
      <div className="flex min-w-0 flex-1 items-center gap-2 text-[0.8rem] leading-5">
        <span className="min-w-0 shrink truncate font-medium" title={issue.title}>
          {issue.title}
        </span>
        {parent ? (
          <span
            className="hidden min-w-0 items-center gap-1 text-muted-foreground md:flex"
            title={`父 Issue：${parent.title}`}
          >
            <ChevronRight className="size-3.5 shrink-0" />
            <span className="truncate">{parent.title}</span>
          </span>
        ) : null}
      </div>
      {children.length > 0 ? (
        <span
          className="hidden shrink-0 rounded-full border bg-background px-2 py-0.5 text-[11px] text-muted-foreground tabular-nums sm:inline-flex"
          title={`${completedChildren} / ${children.length} 个直属子 Issue 已完成`}
        >
          {completedChildren}/{children.length}
        </span>
      ) : null}
      {assignee ? (
        <span
          className="hidden max-w-32 shrink-0 truncate text-xs text-muted-foreground sm:block"
          title={assignee}
        >
          {assignee}
        </span>
      ) : (
        <span
          className="hidden shrink-0 text-xs text-muted-foreground/60 sm:block"
          title="未分配 Agent"
        >
          未分配
        </span>
      )}
      <time
        dateTime={issue.updatedAt}
        title={formatTime(issue.updatedAt)}
        className="hidden w-20 shrink-0 text-right text-xs text-muted-foreground tabular-nums lg:block"
      >
        {formatIssueListDate(issue.updatedAt)}
      </time>
    </div>
  )
}

function formatIssueListDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "short",
    day: "numeric",
  }).format(date)
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
