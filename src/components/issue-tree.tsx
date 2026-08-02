import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { Link } from "react-router-dom"
import {
  CircleCheck,
  ChevronDown,
  CircleDotDashed,
  CircleX,
  GitBranch,
  GitMerge,
  TriangleAlert,
  Workflow,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { AgentDefinition, Execution, Issue, IssueRelation, TaskAgent } from "@/types"

const terminalStatuses = new Set(["done", "failed", "budget_exceeded", "cancelled"])
const activeExecutionStatuses = new Set([
  "queued",
  "starting",
  "running",
  "waiting_approval",
])

interface IssueTreeProps {
  issues: Issue[]
  relations: IssueRelation[]
  agents: AgentDefinition[]
  taskAgents?: TaskAgent[]
  executions?: Execution[]
  rootIds?: string[]
  mode?: "hierarchy" | "dependency"
  defaultExpansion?: "none" | "roots" | "all"
  className?: string
}

export function IssueTree({
  issues,
  relations,
  agents,
  taskAgents = [],
  executions = [],
  rootIds,
  mode = "hierarchy",
  defaultExpansion = "roots",
  className,
}: IssueTreeProps) {
  const issueMap = React.useMemo(
    () => new Map(issues.map((issue) => [issue.id, issue])),
    [issues]
  )
  const childrenMap = React.useMemo(() => {
    const map = new Map<string, Issue[]>()
    for (const issue of issues) {
      if (!issue.parentId) continue
      const list = map.get(issue.parentId) ?? []
      list.push(issue)
      map.set(issue.parentId, list)
    }
    for (const list of map.values()) list.sort((a, b) => a.number - b.number)
    return map
  }, [issues])
  const dependencyMap = React.useMemo(() => {
    const map = new Map<string, Issue[]>()
    for (const relation of relations) {
      if (relation.type !== "blocks") continue
      const blocker = issueMap.get(relation.issueId)
      const blocked = issueMap.get(relation.relatedIssueId)
      if (!blocker || !blocked) continue
      const list = map.get(blocker.id) ?? []
      list.push(blocked)
      map.set(blocker.id, list)
    }
    for (const list of map.values()) list.sort((a, b) => a.number - b.number)
    return map
  }, [issueMap, relations])
  const branchMap = mode === "dependency" ? dependencyMap : childrenMap
  const roots = React.useMemo(() => {
    if (rootIds) {
      return rootIds.flatMap((id) => {
        const issue = issueMap.get(id)
        return issue ? [issue] : []
      })
    }
    if (mode === "dependency") {
      const blockedIds = new Set<string>()
      for (const [blockerId, blockedIssues] of dependencyMap) {
        if (!issueMap.has(blockerId)) continue
        for (const blockedIssue of blockedIssues)
          blockedIds.add(blockedIssue.id)
      }
      const dependencyRoots = issues
        .filter((issue) => !blockedIds.has(issue.id))
        .sort((a, b) => a.number - b.number)
      return dependencyRoots.length > 0
        ? dependencyRoots
        : [...issues].sort((a, b) => a.number - b.number)
    }
    return issues
      .filter((issue) => !issue.parentId || !issueMap.has(issue.parentId))
      .sort((a, b) => b.number - a.number)
  }, [dependencyMap, issueMap, issues, mode, rootIds])
  const agentMap = React.useMemo(
    () => new Map(agents.map((agent) => [agent.id, agent.name])),
    [agents]
  )
  const taskAgentMap = React.useMemo(
    () => new Map(taskAgents.map((identity) => [identity.id, `${identity.name} · ${agentMap.get(identity.agentId) ?? identity.agentId}`])),
    [agentMap, taskAgents]
  )
  const executionMap = React.useMemo(() => {
    const map = new Map<string, Execution>()
    for (const execution of executions) {
      const current = map.get(execution.issueId)
      if (
        !current ||
        new Date(execution.startedAt).getTime() >
          new Date(current.startedAt).getTime()
      ) {
        map.set(execution.issueId, execution)
      }
    }
    return map
  }, [executions])
  const visibleIssues = React.useMemo(() => {
    const ids = new Set<string>()
    const visit = (issue: Issue) => {
      if (ids.has(issue.id)) return
      ids.add(issue.id)
      for (const child of branchMap.get(issue.id) ?? []) visit(child)
    }
    for (const root of roots) visit(root)
    return issues.filter((issue) => ids.has(issue.id))
  }, [branchMap, issues, roots])
  const summary = React.useMemo(
    () => summarizeTree(visibleIssues, executionMap),
    [executionMap, visibleIssues]
  )
  const [expanded, setExpanded] = React.useState<Set<string>>(
    () =>
      new Set(
        (defaultExpansion === "all"
          ? issues
          : defaultExpansion === "roots"
            ? roots
            : []
        ).map((issue) => issue.id)
      )
  )
  const knownIssueIDs = React.useRef(new Set<string>())
  React.useEffect(() => {
    const known = knownIssueIDs.current
    const newlyExpandable =
      defaultExpansion === "all"
        ? issues.filter((issue) => !known.has(issue.id))
        : defaultExpansion === "roots"
          ? roots.filter((issue) => !known.has(issue.id))
          : []
    for (const issue of issues) known.add(issue.id)
    if (newlyExpandable.length === 0) return
    setExpanded((current) => {
      const next = new Set(current)
      for (const issue of newlyExpandable) next.add(issue.id)
      return next
    })
  }, [defaultExpansion, issues, roots])
  const rows = React.useMemo(() => {
    const items: Array<{
      key: string
      issue: Issue
      depth: number
      reference: boolean
    }> = []
    const rendered = new Set<string>()
    const visit = (issue: Issue, depth: number, path: string) => {
      const reference = rendered.has(issue.id)
      items.push({ key: path, issue, depth, reference })
      if (reference || !expanded.has(issue.id)) return
      rendered.add(issue.id)
      for (const child of branchMap.get(issue.id) ?? []) {
        visit(child, depth + 1, `${path}/${child.id}`)
      }
    }
    for (const root of roots) visit(root, 0, root.id)
    return items
  }, [branchMap, expanded, roots])
  const viewportRef = React.useRef<HTMLDivElement>(null)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => viewportRef.current,
    estimateSize: () => 92,
    getItemKey: (index) => rows[index]?.key ?? index,
    overscan: 8,
  })

  if (roots.length === 0) {
    return (
      <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">
        没有匹配的 Issues
      </div>
    )
  }

  return (
    <div className={cn("min-w-0", className)}>
      <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
        <Badge variant={summary.running > 0 ? "default" : "outline"}>
          {summary.running > 0 ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <CircleDotDashed data-icon="inline-start" />
          )}
          执行中 {summary.running}
        </Badge>
        <Badge variant={summary.failed > 0 ? "destructive" : "outline"}>
          <TriangleAlert data-icon="inline-start" />
          失败 {summary.failed}
        </Badge>
        <Badge variant="secondary">
          <CircleCheck data-icon="inline-start" />
          已结束 {summary.completed}
        </Badge>
        <Badge variant="outline">
          <CircleDotDashed data-icon="inline-start" />
          待执行 {summary.pending}
        </Badge>
      </div>
      <ScrollArea
        viewportRef={viewportRef}
        className="h-[min(680px,calc(100dvh-260px))] min-h-[360px]"
      >
        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const row = rows[virtualItem.index]
            if (!row) return null
            return (
              <div
                key={virtualItem.key}
                ref={virtualizer.measureElement}
                data-index={virtualItem.index}
                className="absolute top-0 left-0 w-full border-b"
                style={{ transform: `translateY(${virtualItem.start}px)` }}
              >
                <TreeNode
                  issue={row.issue}
                  depth={row.depth}
                  branchMap={branchMap}
                  hierarchyChildrenMap={childrenMap}
                  issueMap={issueMap}
                  relations={relations}
                  agentMap={agentMap}
                  taskAgentMap={taskAgentMap}
                  executionMap={executionMap}
                  mode={mode}
                  reference={row.reference}
                  open={expanded.has(row.issue.id)}
                  onToggle={() =>
                    setExpanded((current) => {
                      const next = new Set(current)
                      if (next.has(row.issue.id)) next.delete(row.issue.id)
                      else next.add(row.issue.id)
                      return next
                    })
                  }
                />
              </div>
            )
          })}
        </div>
      </ScrollArea>
    </div>
  )
}

interface TreeNodeProps {
  issue: Issue
  depth: number
  branchMap: Map<string, Issue[]>
  hierarchyChildrenMap: Map<string, Issue[]>
  issueMap: Map<string, Issue>
  relations: IssueRelation[]
  agentMap: Map<string, string>
  taskAgentMap: Map<string, string>
  executionMap: Map<string, Execution>
  mode: "hierarchy" | "dependency"
  reference: boolean
  open: boolean
  onToggle: () => void
}

function TreeNode({
  issue,
  depth,
  branchMap,
  hierarchyChildrenMap,
  issueMap,
  relations,
  agentMap,
  taskAgentMap,
  executionMap,
  mode,
  reference,
  open,
  onToggle,
}: TreeNodeProps) {
  const branches = branchMap.get(issue.id) ?? []
  const hierarchyChildren = hierarchyChildrenMap.get(issue.id) ?? []
  const completed = hierarchyChildren.filter((child) =>
    terminalStatuses.has(child.status)
  ).length
  const dependencies = relations
    .filter((relation) => relation.relatedIssueId === issue.id)
    .flatMap((relation) => {
      const blocker = issueMap.get(relation.issueId)
      return blocker ? [blocker] : []
    })
  const progress = hierarchyChildren.length
    ? Math.round((completed / hierarchyChildren.length) * 100)
    : 0
  const execution = executionMap.get(issue.id)
  const operationalState = issueOperationalState(issue, execution)
  const creatorName =
    agentMap.get(issue.createdBy) ??
    (issue.createdBy === "operator"
      ? "操作员"
      : issue.createdBy === "system"
        ? "系统"
        : issue.createdBy || "未知")

  return (
    <div
      className={cn(
        "group flex min-w-0 items-start gap-2 px-4 py-3 transition-colors hover:bg-muted/35",
        depth > 0 && "border-l border-border/70"
      )}
      style={{ paddingLeft: 16 + Math.min(depth, 4) * 20 }}
    >
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center">
        {reference ? (
          <GitMerge className="size-3.5 text-muted-foreground" />
        ) : branches.length ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={open ? "折叠子 Issues" : "展开子 Issues"}
            onClick={onToggle}
          >
            <ChevronDown
              className={cn("transition-transform", !open && "-rotate-90")}
            />
          </Button>
        ) : (
          <CircleDotDashed className="size-3.5 text-muted-foreground/55" />
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <Link
            to={`/issues/${issue.id}`}
            className="min-w-0 flex-1 basis-48 truncate text-sm font-medium hover:underline"
            title={issue.title}
          >
            {issue.title}
          </Link>
          <OperationalBadge state={operationalState} />
          {reference && (
            <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
              关系引用
            </Badge>
          )}
          {issue.validationDisabled && issue.status !== "cancelled" && (
            <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
              跳过验收
            </Badge>
          )}
        </div>

        <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span className="font-mono text-[11px]">{issue.identifier}</span>
          <span>
            {taskAgentMap.get(issue.assigneeTaskAgentId ?? "") ?? agentMap.get(issue.assigneeAgentId ?? "") ?? "未分配 Agent"}
          </span>
          <span>创建者 {creatorName}</span>
          <span>创建 {formatTime(issue.createdAt)}</span>
          {mode === "hierarchy" ? (
            <>
              <span>深度 {issue.requestDepth}</span>
              {hierarchyChildren.length > 0 && (
                <span>
                  {completed}/{hierarchyChildren.length} 个直属子项完成
                </span>
              )}
            </>
          ) : (
            <>
              <span>{dependencies.length} 个前置依赖</span>
              <span>{branches.length} 个被阻塞项</span>
            </>
          )}
        </div>
        {operationalState.detail && (
          <Tooltip>
            <TooltipTrigger className="mt-1.5 flex max-w-full items-center gap-1 text-xs text-destructive">
              <TriangleAlert className="size-3 shrink-0" />
              <span className="truncate">{operationalState.detail}</span>
            </TooltipTrigger>
            <TooltipContent className="max-w-sm">
              {operationalState.detail}
            </TooltipContent>
          </Tooltip>
        )}
        {mode === "hierarchy" && hierarchyChildren.length > 0 && (
          <Progress
            value={progress}
            className="mt-2 max-w-xs"
            aria-label="子 Issue 完成进度"
          />
        )}
      </div>

      <Badge variant="outline" className="mt-0.5 hidden shrink-0 gap-1 sm:flex">
        {mode === "dependency" ? (
          <Workflow className="size-3" />
        ) : (
          <GitBranch className="size-3" />
        )}
        {branches.length}
      </Badge>
    </div>
  )
}

type OperationalState = {
  kind: "running" | "failed" | "completed" | "cancelled" | "pending"
  label: string
  detail?: string
}

function issueOperationalState(
  issue: Issue,
  execution: Execution | undefined
): OperationalState {
  if (issue.status === "done") {
    return { kind: "completed", label: "任务结束" }
  }
  if (issue.status === "cancelled") {
    return {
      kind: "cancelled",
      label: issue.objectiveAbandoned ? "目标已放弃" : "任务已取消",
    }
  }
  if (issue.status === "budget_exceeded") {
    return { kind: "failed", label: "执行预算已耗尽", detail: issue.error }
  }
  if (issue.executionPhase === "recovering") {
    return { kind: "running", label: "重启恢复中" }
  }
  if (
    issue.status === "blocked" ||
    execution?.status === "failed" ||
    execution?.status === "disconnected"
  ) {
    return {
      kind: "failed",
      label: execution?.status === "disconnected" ? "执行断开" : "执行失败",
      detail: issue.error || execution?.error,
    }
  }
  if (issue.executionPhase === "waiting_children") {
    return { kind: "running", label: "等待子任务" }
  }
  if (issue.executionPhase === "resuming") {
    return { kind: "running", label: "汇总结果" }
  }
  if (issue.executionPhase === "validating") {
    return { kind: "running", label: "验收中" }
  }
  if (issue.executionPhase === "summarizing") {
    return { kind: "running", label: "取消后总结中" }
  }
  if (issue.executionPhase === "budget_summarizing") {
    return { kind: "running", label: "预算耗尽后总结中" }
  }
  if (issue.status === "in_progress") {
    if (execution?.status === "queued") {
      return { kind: "running", label: "执行排队中" }
    }
    if (execution?.status === "starting") {
      return { kind: "running", label: "Agent 启动中" }
    }
    if (execution?.status === "waiting_approval") {
      return { kind: "running", label: "等待审批" }
    }
    return {
      kind: "running",
      label: execution?.currentTool
        ? `执行中 · ${execution.currentTool}`
        : "Agent 执行中",
    }
  }
  if (issue.status === "in_review") {
    return { kind: "pending", label: "等待人工复核" }
  }
  return { kind: "pending", label: "等待调度" }
}

function OperationalBadge({ state }: { state: OperationalState }) {
  if (state.kind === "running") {
    return (
      <Badge className="h-5 px-1.5 text-[10px]">
        <Spinner data-icon="inline-start" />
        {state.label}
      </Badge>
    )
  }
  if (state.kind === "failed") {
    return (
      <Badge variant="destructive" className="h-5 px-1.5 text-[10px]">
        <TriangleAlert data-icon="inline-start" />
        {state.label}
      </Badge>
    )
  }
  if (state.kind === "completed") {
    return (
      <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
        <CircleCheck data-icon="inline-start" />
        {state.label}
      </Badge>
    )
  }
  if (state.kind === "cancelled") {
    return (
      <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
        <CircleX data-icon="inline-start" />
        {state.label}
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
      <CircleDotDashed data-icon="inline-start" />
      {state.label}
    </Badge>
  )
}

function summarizeTree(issues: Issue[], executionMap: Map<string, Execution>) {
  const summary = { running: 0, failed: 0, completed: 0, pending: 0 }
  for (const issue of issues) {
    const execution = executionMap.get(issue.id)
    if (terminalStatuses.has(issue.status)) {
      summary.completed++
    } else if (
      issue.status === "blocked" ||
      execution?.status === "failed" ||
      execution?.status === "disconnected"
    ) {
      summary.failed++
    } else if (
      issue.executionPhase === "recovering" ||
      issue.status === "in_progress" ||
      (execution && activeExecutionStatuses.has(execution.status))
    ) {
      summary.running++
    } else {
      summary.pending++
    }
  }
  return summary
}
