import * as React from "react"
import { LayoutGrid, Plus } from "lucide-react"
import { useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { CreateIssueDialog } from "@/components/create-issue-dialog"
import { CopyIssueIdentifier } from "@/components/copy-issue-identifier"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { IssueStatusSelect } from "@/components/issue-status-select"
import { SearchInput } from "@/components/ui/search-input"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { issuesForTask } from "@/lib/collections"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import {
  issueLabelsOf,
  issueLabelMeta,
  issueWorkflowStatus,
  workflowStatusLabels,
} from "@/lib/issue-workflow"
import { updateIssue } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { Issue, IssueRuntimeView, IssueStatus } from "@/types"

type BoardColumn = {
  status: IssueStatus
  dot: string
  hint: string
}

const columns: BoardColumn[] = [
  {
    status: "todo",
    dot: "bg-info",
    hint: "已就绪、等待开工",
  },
  {
    status: "in_progress",
    dot: "bg-success",
    hint: "Agent 正在执行",
  },
  {
    status: "in_review",
    dot: "bg-warning",
    hint: "等待人工复核结果",
  },
  {
    status: "done",
    dot: "bg-muted-foreground",
    hint: "目标已达成",
  },
  {
    status: "cancelled",
    dot: "bg-muted-foreground/40",
    hint: "已终止或放弃",
  },
]

const priorityRank: Record<Issue["priority"], number> = {
  high: 0,
  middle: 1,
  low: 2,
}

const priorityDot: Record<Issue["priority"], string> = {
  high: "bg-destructive",
  middle: "bg-warning",
  low: "bg-info",
}

const priorityLabel: Record<Issue["priority"], string> = {
  high: "高优先级",
  middle: "中优先级",
  low: "低优先级",
}

export function TaskBoardPage() {
  const { state, setState, refresh } = useAppState()
  const { taskId } = useParams()
  const [query, setQuery] = React.useState("")
  const [draggingId, setDraggingId] = React.useState<string | null>(null)
  const [dropTarget, setDropTarget] = React.useState<IssueStatus | null>(null)
  const [createStatus, setCreateStatus] = React.useState<IssueStatus | null>(
    null
  )

  const allIssues = React.useMemo(() => state?.issues ?? [], [state?.issues])
  const deferredQuery = React.useDeferredValue(query)
  const view = React.useMemo(() => {
    const task = taskId
      ? state?.tasks.find((item) => item.id === taskId)
      : undefined
    const scopedIssues = task ? issuesForTask(task.id, allIssues) : []
    const normalizedQuery = deferredQuery.trim().toLocaleLowerCase()
    const issueById = new Map(scopedIssues.map((issue) => [issue.id, issue]))
    const childCountByParent = new Map<string, number>()
    const itemsByStatus = new Map<IssueStatus, Issue[]>(
      columns.map((column) => [column.status, []])
    )
    const totalByStatus = new Map<IssueStatus, number>()

    for (const issue of scopedIssues) {
      if (issue.parentId) {
        childCountByParent.set(
          issue.parentId,
          (childCountByParent.get(issue.parentId) ?? 0) + 1
        )
      }
      const status = issueWorkflowStatus(issue.status)
      totalByStatus.set(status, (totalByStatus.get(status) ?? 0) + 1)
      if (
        !normalizedQuery ||
        `${issue.identifier} ${issue.title}`
          .toLocaleLowerCase()
          .includes(normalizedQuery)
      ) {
        itemsByStatus.get(status)?.push(issue)
      }
    }
    for (const items of itemsByStatus.values()) {
      items.sort(
        (left, right) =>
          priorityRank[left.priority] - priorityRank[right.priority] ||
          left.createdAt.localeCompare(right.createdAt)
      )
    }

    return {
      taskSourceId: task?.id,
      issueById,
      childCountByParent,
      itemsByStatus,
      totalByStatus,
      runtimeMap: issueRuntimeMap(state?.issueRuntimes ?? []),
      task,
      agentById: new Map(
        (state?.agents ?? []).map((agent) => [agent.id, agent])
      ),
    }
  }, [
    allIssues,
    deferredQuery,
    state?.agents,
    state?.issueRuntimes,
    state?.tasks,
    taskId,
  ])
  const { taskSourceId } = view

  const moveIssue = async (issueId: string, status: IssueStatus) => {
    const previous = view.issueById.get(issueId)
    setState((current) =>
      current
        ? {
            ...current,
            issues: current.issues.map((issue) =>
              issue.id === issueId ? { ...issue, status } : issue
            ),
          }
        : current
    )
    try {
      await updateIssue(issueId, { status })
      toast.success(`已移动到「${workflowStatusLabels[status]}」`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "移动 Issue 失败")
      if (previous) {
        setState((current) =>
          current
            ? {
                ...current,
                issues: current.issues.map((issue) =>
                  issue.id === issueId
                    ? { ...issue, status: previous.status }
                    : issue
                ),
              }
            : current
        )
      }
      void refresh()
    }
  }

  const beginDrag = React.useCallback(
    (issueId: string, event: React.DragEvent) => {
      event.dataTransfer.setData("text/plain", issueId)
      event.dataTransfer.effectAllowed = "move"
      setDraggingId(issueId)
    },
    []
  )
  const endDrag = React.useCallback(() => {
    setDraggingId(null)
    setDropTarget(null)
  }, [])

  return (
    <div className="flex size-full min-h-0 flex-col gap-3">
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <SearchInput
          value={query}
          onChange={setQuery}
          ariaLabel="搜索看板 Issue"
          placeholder="搜索编号或标题…"
        />
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">
            {view.task?.title ?? "看板"}
          </p>
        </div>
        <p className="ml-auto hidden shrink-0 items-center gap-1.5 text-xs text-muted-foreground md:flex">
          <LayoutGrid className="size-3.5" />
          点击卡片上的状态可直接修改，或拖拽卡片换列
        </p>
      </div>

      <div className="flex min-h-0 flex-1 gap-3 overflow-x-auto pb-2">
        {columns.map((column) => {
          const items = view.itemsByStatus.get(column.status) ?? []
          const isDropTarget = dropTarget === column.status
          const total = view.totalByStatus.get(column.status) ?? 0
          return (
            <section
              key={column.status}
              aria-label={workflowStatusLabels[column.status]}
              onDragOver={(event) => {
                event.preventDefault()
                event.dataTransfer.dropEffect = "move"
                setDropTarget(column.status)
              }}
              onDragLeave={(event) => {
                if (
                  event.currentTarget.contains(
                    event.relatedTarget as Node | null
                  )
                ) {
                  return
                }
                setDropTarget((current) =>
                  current === column.status ? null : current
                )
              }}
              onDrop={(event) => {
                event.preventDefault()
                setDropTarget(null)
                const issueId =
                  draggingId ?? event.dataTransfer.getData("text/plain")
                if (!issueId) return
                const issue = view.issueById.get(issueId)
                if (!issue) return
                const target = issueWorkflowStatus(issue.status)
                if (target === column.status) return
                void moveIssue(issueId, column.status)
              }}
              className={cn(
                "flex w-72 shrink-0 flex-col rounded-xl border bg-muted/20 transition-[background-color,border-color,box-shadow] duration-150",
                isDropTarget &&
                  "border-primary/50 bg-primary/5 ring-1 ring-primary/30"
              )}
            >
              <header className="flex h-10 shrink-0 items-center gap-2 px-3">
                <span className={cn("size-2 rounded-full", column.dot)} />
                <h2 className="text-sm font-medium">
                  {workflowStatusLabels[column.status]}
                </h2>
                <span className="ml-auto rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground tabular-nums">
                  {total}
                </span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`在「${workflowStatusLabels[column.status]}」中新建 Issue`}
                  disabled={!taskSourceId}
                  onClick={() => {
                    if (taskSourceId) setCreateStatus(column.status)
                  }}
                  className="rounded-md text-muted-foreground hover:bg-background hover:text-foreground"
                >
                  <Plus />
                </Button>
              </header>
              <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">
                {items.length === 0 ? (
                  <div className="flex min-h-20 flex-1 items-center justify-center rounded-lg border border-dashed border-foreground/10 px-3 py-4 text-center">
                    <p className="text-xs text-muted-foreground">
                      {isDropTarget ? "松手放到这里" : column.hint}
                    </p>
                  </div>
                ) : (
                  items.map((issue) => (
                    <BoardCard
                      key={issue.id}
                      issue={issue}
                      runtime={issueRuntimeOrUnavailable(
                        view.runtimeMap,
                        issue.id
                      )}
                      childCount={view.childCountByParent.get(issue.id) ?? 0}
                      assigneeName={
                        issue.assigneeAgentId
                          ? (view.agentById.get(issue.assigneeAgentId)?.name ??
                            undefined)
                          : undefined
                      }
                      dragging={draggingId === issue.id}
                      onDragStart={beginDrag}
                      onDragEnd={endDrag}
                    />
                  ))
                )}
              </div>
            </section>
          )
        })}
      </div>

      {taskSourceId ? (
        <CreateIssueDialog
          open={createStatus !== null}
          onOpenChange={(open) => {
            if (!open) setCreateStatus(null)
          }}
          defaultStatus={createStatus ?? "todo"}
          workspace={view.task?.workspace}
          taskSourceId={taskSourceId}
        />
      ) : null}
    </div>
  )
}

const BoardCard = React.memo(function BoardCard({
  issue,
  runtime,
  childCount,
  assigneeName,
  dragging,
  onDragStart,
  onDragEnd,
}: {
  issue: Issue
  runtime: IssueRuntimeView
  childCount: number
  assigneeName?: string
  dragging: boolean
  onDragStart: (issueId: string, event: React.DragEvent) => void
  onDragEnd: () => void
}) {
  const navigate = useNavigate()
  const labels = issueLabelsOf(issue)
  return (
    <article
      draggable
      role="link"
      tabIndex={0}
      aria-label={`${issue.identifier} ${issue.title}`}
      onDragStart={(event) => onDragStart(issue.id, event)}
      onDragEnd={onDragEnd}
      onClick={(event) => {
        if ((event.target as HTMLElement).closest("button, a, input, select, textarea, [role='menuitem']")) return
        navigate(`/issues/${issue.id}?view=board`)
      }}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          navigate(`/issues/${issue.id}?view=board`)
        }
      }}
      className={cn(
        "group flex cursor-pointer flex-col gap-1.5 rounded-lg border bg-card p-3 shadow-sm ring-1 ring-foreground/5 transition-[box-shadow,opacity,transform] duration-150 hover:-translate-y-px hover:shadow-md hover:ring-foreground/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60 active:cursor-grabbing",
        dragging && "translate-y-0 opacity-45"
      )}
    >
      <div className="flex items-center gap-1.5">
        <CopyIssueIdentifier identifier={issue.identifier} />
        <span
          className={cn("size-1.5 rounded-full", priorityDot[issue.priority])}
          title={priorityLabel[issue.priority]}
        />
        <IssueStatusSelect issue={issue} className="ml-auto" />
      </div>
      <p className="line-clamp-2 text-sm leading-snug font-medium">
        {issue.title}
      </p>
      {labels.length > 0 ? (
        <div className="flex flex-wrap items-center gap-1">
          {labels.map((label) => (
            <Badge
              key={label}
              variant="outline"
              className={cn(
                "h-5 px-1.5 text-[10px] font-medium",
                issueLabelMeta[label].className
              )}
            >
              {issueLabelMeta[label].label}
            </Badge>
          ))}
        </div>
      ) : null}
      <div className="flex items-center gap-1.5">
        <IssueRuntimeBadge runtime={runtime} />
        {childCount > 0 ? (
          <Badge
            variant="outline"
            className="h-5 px-1.5 text-[10px] font-normal text-muted-foreground"
          >
            {childCount} 个子项
          </Badge>
        ) : null}
        {assigneeName ? (
          <span className="ml-auto max-w-24 truncate rounded-md bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {assigneeName}
          </span>
        ) : null}
      </div>
    </article>
  )
})
