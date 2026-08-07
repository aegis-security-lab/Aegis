import * as React from "react"
import {
  ChevronRight,
  ChevronsDown,
  ChevronsUp,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"

import { IssueTree } from "@/components/issue-tree"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { SearchInput } from "@/components/ui/search-input"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import { issueWorkflowStatus } from "@/lib/issue-workflow"
import type { Issue, IssueRuntimeView, IssueStatus } from "@/types"

const statuses: IssueStatus[] = [
  "todo",
  "in_progress",
  "in_review",
  "done",
  "cancelled",
]

const statusLabels: Record<IssueStatus, string> = {
  backlog: "待处理",
  todo: "待执行",
  in_progress: "处理中",
  in_review: "待复核",
  blocked: "阻塞",
  failed: "失败",
  budget_exceeded: "超出预算",
  done: "已完成",
  cancelled: "已取消",
}

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

export function IssuesPage() {
  const { state } = useAppState()
  const { issueId: taskRootId } = useParams()
  const [filter, setFilter] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [expansion, setExpansion] = React.useState<"none" | "roots" | "all">(
    "roots"
  )
  const [expansionVersion, setExpansionVersion] = React.useState(0)
  const [viewMode, setViewMode] = React.useState<ViewMode>("tree")
  const [openGroups, setOpenGroups] = React.useState<
    Record<IssueGroupKey, boolean>
  >(() =>
    Object.fromEntries(groupOrder.map((key) => [key, true])) as Record<
      IssueGroupKey,
      boolean
    >
  )
  const allIssues = state?.issues ?? []
  const scopedIssues = taskRootId
    ? issuesInSubtree(taskRootId, allIssues)
    : allIssues
  const matched = scopedIssues.filter(
    (issue) =>
      (filter === "all" || issueWorkflowStatus(issue.status) === filter) &&
      (!query.trim() ||
        `${issue.identifier} ${issue.title}`
          .toLocaleLowerCase()
          .includes(query.trim().toLocaleLowerCase()))
  )
  const hierarchyIssues = includeAncestors(matched, scopedIssues)
  const displayedIssues = hierarchyIssues
  const runtimeMap = issueRuntimeMap(state?.issueRuntimes ?? [])
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
          <ToggleGroupItem value="all">全部</ToggleGroupItem>
          {statuses.map((status) => (
            <ToggleGroupItem key={status} value={status}>
              {statusLabels[status]}
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
                <button
                  type="button"
                  aria-expanded={open}
                  onClick={() =>
                    setOpenGroups((current) => ({
                      ...current,
                      [key]: !current[key],
                    }))
                  }
                  className="flex h-8 w-full items-center gap-2 rounded-md px-1 text-xs font-medium text-muted-foreground hover:bg-muted/40 hover:text-foreground"
                >
                  <ChevronRight
                    className={cn(
                      "size-3.5 transition-transform",
                      open && "rotate-90"
                    )}
                  />
                  {groupLabels[key]}
                  <span className="tabular-nums">{items.length}</span>
                </button>
                {open ? (
                  <div className="flex flex-col">
                    {items.length === 0 ? (
                      <p className="px-7 py-2 text-xs text-muted-foreground">
                        暂无
                      </p>
                    ) : (
                      items.map((issue) => (
                        <Link
                          key={issue.id}
                          to={`/issues/${issue.id}`}
                          className="flex min-h-9 items-center gap-3 rounded-md px-7 py-1 text-sm hover:bg-muted/40"
                        >
                          <span className="w-20 shrink-0 font-mono text-[11px] text-muted-foreground">
                            {issue.identifier}
                          </span>
                          <span className="min-w-0 flex-1 truncate">
                            {issue.title}
                          </span>
                          <IssueRuntimeBadge
                            runtime={issueRuntimeOrUnavailable(
                              runtimeMap,
                              issue.id
                            )}
                          />
                        </Link>
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
              className="h-full"
            />
          </Card>
        </div>
      )}
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

function issuesInSubtree(rootId: string, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  const result: Issue[] = []
  const seen = new Set<string>()
  const visit = (id: string) => {
    if (seen.has(id)) return
    seen.add(id)
    const issue = byID.get(id)
    if (!issue) return
    result.push(issue)
    for (const candidate of issues) {
      if (candidate.parentId === id) visit(candidate.id)
    }
  }
  visit(rootId)
  return result
}

function classifyIssue(
  issue: Issue,
  runtime: IssueRuntimeView
): IssueGroupKey {
  if (["completed", "cancelled"].includes(runtime.kind)) return "finished"
  if (runtime.kind === "failed") return "failed"
  if (runtime.kind === "running") return "running"
  if (runtime.kind === "waiting") return "waiting"
  if (["done", "cancelled"].includes(issue.status)) return "finished"
  if (["failed", "budget_exceeded", "blocked"].includes(issue.status)) {
    return "failed"
  }
  return "todo"
}
