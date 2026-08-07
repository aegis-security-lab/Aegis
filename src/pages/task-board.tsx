import * as React from "react"
import { LayoutGrid, Plus } from "lucide-react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"

import { CreateIssueDialog } from "@/components/create-issue-dialog"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { SearchInput } from "@/components/ui/search-input"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { issuesInSubtree } from "@/lib/collections"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { updateIssue } from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { Issue, IssueRuntimeView, IssueStatus } from "@/types"

type BoardColumn = {
  status: IssueStatus
  label: string
  dot: string
  hint: string
}

const columns: BoardColumn[] = [
  {
    status: "backlog",
    label: "待处理",
    dot: "bg-muted-foreground",
    hint: "还没有安排进迭代的诉求",
  },
  {
    status: "todo",
    label: "待执行",
    dot: "bg-info",
    hint: "已就绪、等待开工",
  },
  {
    status: "in_progress",
    label: "处理中",
    dot: "bg-success",
    hint: "Agent 正在执行",
  },
  {
    status: "in_review",
    label: "待复核",
    dot: "bg-warning",
    hint: "等待人工复核结果",
  },
  {
    status: "blocked",
    label: "阻塞",
    dot: "bg-destructive",
    hint: "执行被依赖或错误阻塞",
  },
  {
    status: "failed",
    label: "失败",
    dot: "bg-destructive",
    hint: "执行失败，需要处理",
  },
  {
    status: "budget_exceeded",
    label: "超出预算",
    dot: "bg-warning",
    hint: "执行预算已耗尽",
  },
  {
    status: "done",
    label: "已完成",
    dot: "bg-muted-foreground",
    hint: "目标已达成",
  },
  {
    status: "cancelled",
    label: "已取消",
    dot: "bg-muted-foreground/40",
    hint: "已终止或放弃",
  },
]

const columnByStatus = new Map(columns.map((column) => [column.status, column]))

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
  const { issueId: taskRootId } = useParams()
  const [query, setQuery] = React.useState("")
  const [draggingId, setDraggingId] = React.useState<string | null>(null)
  const [dropTarget, setDropTarget] = React.useState<IssueStatus | null>(null)
  const [createStatus, setCreateStatus] = React.useState<IssueStatus | null>(
    null
  )

  const allIssues = state?.issues ?? []
  const scopedIssues = taskRootId
    ? issuesInSubtree(taskRootId, allIssues)
    : allIssues
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const matched = scopedIssues.filter(
    (issue) =>
      !normalizedQuery ||
      `${issue.identifier} ${issue.title}`
        .toLocaleLowerCase()
        .includes(normalizedQuery)
  )
  const issueById = new Map(scopedIssues.map((issue) => [issue.id, issue]))
  const childCountByParent = new Map<string, number>()
  for (const issue of scopedIssues) {
    if (issue.parentId) {
      childCountByParent.set(
        issue.parentId,
        (childCountByParent.get(issue.parentId) ?? 0) + 1
      )
    }
  }
  const runtimeMap = issueRuntimeMap(state?.issueRuntimes ?? [])
  const rootIssue = taskRootId
    ? allIssues.find((issue) => issue.id === taskRootId)
    : undefined
  const task = rootIssue?.taskSourceId
    ? state?.tasks.find((item) => item.id === rootIssue.taskSourceId)
    : undefined
  const agentById = new Map(
    (state?.agents ?? []).map((agent) => [agent.id, agent])
  )

  const moveIssue = async (issueId: string, status: IssueStatus) => {
    const previous = allIssues.find((issue) => issue.id === issueId)
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
      toast.success(`已移动到「${columnByStatus.get(status)?.label ?? status}」`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "移动 Issue 失败")
      if (previous) {
        setState((current) =>
          current
            ? {
                ...current,
                issues: current.issues.map((issue) =>
                  issue.id === issueId ? { ...issue, status: previous.status } : issue
                ),
              }
            : current
        )
      }
      void refresh()
    }
  }

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
            {task ? task.title : (rootIssue?.identifier ?? "看板")}
          </p>
        </div>
        <p className="ml-auto hidden shrink-0 items-center gap-1.5 text-xs text-muted-foreground md:flex">
          <LayoutGrid className="size-3.5" />
          拖拽卡片即可在状态列之间移动
        </p>
      </div>

      <div className="flex min-h-0 flex-1 gap-3 overflow-x-auto pb-2">
        {columns.map((column) => {
          const items = matched
            .filter((issue) => issue.status === column.status)
            .sort(
              (left, right) =>
                priorityRank[left.priority] - priorityRank[right.priority] ||
                left.createdAt.localeCompare(right.createdAt)
            )
          const isDropTarget = dropTarget === column.status
          const total = scopedIssues.filter(
            (issue) => issue.status === column.status
          ).length
          return (
            <section
              key={column.status}
              aria-label={column.label}
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
                const issue = issueById.get(issueId)
                if (!issue || issue.status === column.status) return
                void moveIssue(issueId, column.status)
              }}
              className={cn(
                "flex w-72 shrink-0 flex-col rounded-xl border bg-muted/20",
                isDropTarget &&
                  "border-primary/50 bg-primary/5 ring-1 ring-primary/30"
              )}
            >
              <header className="flex h-10 shrink-0 items-center gap-2 px-3">
                <span className={cn("size-2 rounded-full", column.dot)} />
                <h2 className="text-sm font-medium">{column.label}</h2>
                <span className="ml-auto rounded-md bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">
                  {total}
                </span>
                <button
                  type="button"
                  aria-label={`在「${column.label}」中新建 Issue`}
                  title={`在「${column.label}」中新建 Issue`}
                  onClick={() => setCreateStatus(column.status)}
                  className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                >
                  <Plus className="size-4" />
                </button>
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
                      runtime={issueRuntimeOrUnavailable(runtimeMap, issue.id)}
                      childCount={childCountByParent.get(issue.id) ?? 0}
                      assigneeName={
                        issue.assigneeAgentId
                          ? (agentById.get(issue.assigneeAgentId)?.name ?? undefined)
                          : undefined
                      }
                      dragging={draggingId === issue.id}
                      onDragStart={(event) => {
                        event.dataTransfer.setData("text/plain", issue.id)
                        event.dataTransfer.effectAllowed = "move"
                        setDraggingId(issue.id)
                      }}
                      onDragEnd={() => {
                        setDraggingId(null)
                        setDropTarget(null)
                      }}
                    />
                  ))
                )}
              </div>
            </section>
          )
          })}
      </div>
      <CreateIssueDialog
        open={createStatus !== null}
        onOpenChange={(open) => !open && setCreateStatus(null)}
        defaultStatus={createStatus ?? "todo"}
        workspace={rootIssue?.workspace}
        parentId={taskRootId}
      />
    </div>
  )
}

function BoardCard({
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
  onDragStart: (event: React.DragEvent) => void
  onDragEnd: () => void
}) {
  return (
    <Link
      to={`/issues/${issue.id}?view=board`}
      draggable
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      className={cn(
        "group flex cursor-grab flex-col gap-1.5 rounded-lg border bg-card p-3 shadow-sm ring-1 ring-foreground/5 transition-[box-shadow,opacity,transform] hover:shadow-md hover:ring-foreground/15 active:cursor-grabbing",
        dragging && "opacity-50"
      )}
    >
      <div className="flex items-center gap-2">
        <span className="font-mono text-[11px] text-muted-foreground">
          {issue.identifier}
        </span>
        <span
          className={cn(
            "size-1.5 rounded-full",
            priorityDot[issue.priority],
            issue.parentId && "ml-1"
          )}
          title={priorityLabel[issue.priority]}
        />
        {issue.parentId ? (
          <Badge variant="outline" className="h-4 px-1 text-[10px] font-normal text-muted-foreground">
            子任务
          </Badge>
        ) : null}
      </div>
      <p className="line-clamp-2 text-sm leading-snug">{issue.title}</p>
      <div className="flex items-center gap-1.5">
        <IssueRuntimeBadge runtime={runtime} />
        {childCount > 0 ? (
          <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal text-muted-foreground">
            {childCount} 个子项
          </Badge>
        ) : null}
        {assigneeName ? (
          <span className="ml-auto max-w-24 truncate rounded-md bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {assigneeName}
          </span>
        ) : null}
      </div>
    </Link>
  )
}
