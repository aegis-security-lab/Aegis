import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { useNavigate } from "react-router-dom"
import {
  ChevronDown,
  CircleDotDashed,
  GitBranch,
  GitMerge,
  TriangleAlert,
  Workflow,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { CopyIssueIdentifier } from "@/components/copy-issue-identifier"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { formatTime } from "@/lib/format"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { issueWorkflowStatus } from "@/lib/issue-workflow"
import { IssueStatusSelect } from "@/components/issue-status-select"
import { cn } from "@/lib/utils"
import type {
  AgentDefinition,
  Issue,
  IssueRelation,
  IssueRuntimeView,
  TaskAgent,
} from "@/types"

const terminalStatuses = new Set(["done", "cancelled"])
interface IssueTreeProps {
  issues: Issue[]
  relations: IssueRelation[]
  agents: AgentDefinition[]
  taskAgents?: TaskAgent[]
  runtimes?: IssueRuntimeView[]
  rootIds?: string[]
  mode?: "hierarchy" | "dependency"
  defaultExpansion?: "none" | "roots" | "all"
  editableStatus?: boolean
  className?: string
}

export function IssueTree({
  issues,
  relations,
  agents,
  taskAgents = [],
  runtimes = [],
  rootIds,
  mode = "hierarchy",
  defaultExpansion = "roots",
  editableStatus = false,
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
  const dependenciesByIssue = React.useMemo(() => {
    const map = new Map<string, Issue[]>()
    for (const relation of relations) {
      const blocker = issueMap.get(relation.issueId)
      if (!blocker) continue
      const dependencies = map.get(relation.relatedIssueId)
      if (dependencies) dependencies.push(blocker)
      else map.set(relation.relatedIssueId, [blocker])
    }
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
    () =>
      new Map(
        taskAgents.map((identity) => [
          identity.id,
          `${identity.name} · ${agentMap.get(identity.agentId) ?? identity.agentId}`,
        ])
      ),
    [agentMap, taskAgents]
  )
  const runtimeMap = React.useMemo(() => issueRuntimeMap(runtimes), [runtimes])
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
  const toggleIssue = React.useCallback((issueId: string) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(issueId)) next.delete(issueId)
      else next.add(issueId)
      return next
    })
  }, [])
  const viewportRef = React.useRef<HTMLDivElement>(null)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => viewportRef.current,
    estimateSize: () => 60,
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
    <div className={cn("flex min-h-0 flex-col", className)}>
      <ScrollArea viewportRef={viewportRef} className="min-h-[360px] flex-1">
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
                  dependenciesByIssue={dependenciesByIssue}
                  agentMap={agentMap}
                  taskAgentMap={taskAgentMap}
                  runtimeMap={runtimeMap}
                  mode={mode}
                  reference={row.reference}
                  editableStatus={editableStatus}
                  open={expanded.has(row.issue.id)}
                  onToggle={toggleIssue}
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
  dependenciesByIssue: Map<string, Issue[]>
  agentMap: Map<string, string>
  taskAgentMap: Map<string, string>
  runtimeMap: Map<string, IssueRuntimeView>
  mode: "hierarchy" | "dependency"
  reference: boolean
  editableStatus: boolean
  open: boolean
  onToggle: (issueId: string) => void
}

function TreeNode({
  issue,
  depth,
  branchMap,
  hierarchyChildrenMap,
  dependenciesByIssue,
  agentMap,
  taskAgentMap,
  runtimeMap,
  mode,
  reference,
  editableStatus,
  open,
  onToggle,
}: TreeNodeProps) {
  const navigate = useNavigate()
  const branches = branchMap.get(issue.id) ?? []
  const hierarchyChildren = hierarchyChildrenMap.get(issue.id) ?? []
  const completed = hierarchyChildren.filter((child) =>
    terminalStatuses.has(child.status)
  ).length
  const dependencies = dependenciesByIssue.get(issue.id) ?? []
  const progress = hierarchyChildren.length
    ? Math.round((completed / hierarchyChildren.length) * 100)
    : 0
  const runtime = issueRuntimeOrUnavailable(runtimeMap, issue.id)
  const creatorName =
    agentMap.get(issue.createdBy) ??
    (issue.createdBy === "operator"
      ? "操作员"
      : issue.createdBy === "system"
        ? "系统"
        : issue.createdBy || "未知")

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={`打开 ${issue.identifier} ${issue.title}`}
      onClick={(event) => {
        if ((event.target as HTMLElement).closest("button, a, input, select, textarea, [role='menuitem']")) return
        navigate(`/issues/${issue.id}`)
      }}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          navigate(`/issues/${issue.id}`)
        }
      }}
      className={cn(
        "group flex min-w-0 cursor-pointer items-start gap-1.5 px-4 py-1 outline-none transition-colors hover:bg-muted/35 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/60",
        depth > 0 && "border-l border-border/70"
      )}
      style={{ paddingLeft: 16 + Math.min(depth, 4) * 20 }}
    >
      <div className="flex size-7 shrink-0 items-center justify-center">
        {reference ? (
          <GitMerge className="size-3.5 text-muted-foreground" />
        ) : branches.length ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={open ? "折叠子 Issues" : "展开子 Issues"}
            onClick={() => onToggle(issue.id)}
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
          <span
            className="min-w-0 flex-1 basis-48 truncate text-sm font-medium"
            title={issue.title}
          >
            {issue.title}
          </span>
          <IssueRuntimeBadge runtime={runtime} />
          {editableStatus ? (
            <IssueStatusSelect issue={issue} />
          ) : (
            <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
              {issueWorkflowStatus(issue.status)}
            </Badge>
          )}
          <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
            {issue.priority}
          </Badge>
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

        <div className="mt-1 flex min-w-0 flex-nowrap items-center gap-x-3 overflow-hidden text-xs text-muted-foreground [&>span]:shrink-0">
          <CopyIssueIdentifier identifier={issue.identifier} />
          <span>
            {taskAgentMap.get(issue.assigneeTaskAgentId ?? "") ??
              agentMap.get(issue.assigneeAgentId ?? "") ??
              "未分配 Agent"}
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
        {runtime.detail && runtime.health !== "healthy" && (
          <Tooltip>
            <TooltipTrigger className="mt-1.5 flex max-w-full items-center gap-1 text-xs text-destructive">
              <TriangleAlert className="size-3 shrink-0" />
              <span className="truncate">{runtime.detail}</span>
            </TooltipTrigger>
            <TooltipContent className="max-w-sm">
              {runtime.detail}
            </TooltipContent>
          </Tooltip>
        )}
        {mode === "hierarchy" && hierarchyChildren.length > 0 && (
          <Progress
            value={progress}
            className="mt-1.5 max-w-xs"
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
