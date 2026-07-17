import * as React from "react"
import { Link } from "react-router-dom"
import {
  ChevronDown,
  CircleDotDashed,
  GitBranch,
  LockKeyhole,
  RotateCcw,
} from "lucide-react"

import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Progress } from "@/components/ui/progress"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import type { AgentDefinition, Issue, IssueRelation } from "@/types"

const terminalStatuses = new Set(["done", "cancelled"])

interface IssueTreeProps {
  issues: Issue[]
  relations: IssueRelation[]
  agents: AgentDefinition[]
  rootIds?: string[]
  className?: string
}

export function IssueTree({
  issues,
  relations,
  agents,
  rootIds,
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
  const roots = React.useMemo(() => {
    if (rootIds) {
      return rootIds.flatMap((id) => {
        const issue = issueMap.get(id)
        return issue ? [issue] : []
      })
    }
    return issues
      .filter((issue) => !issue.parentId || !issueMap.has(issue.parentId))
      .sort((a, b) => b.number - a.number)
  }, [issueMap, issues, rootIds])
  const agentMap = React.useMemo(
    () => new Map(agents.map((agent) => [agent.id, agent.name])),
    [agents]
  )

  if (roots.length === 0) {
    return (
      <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">
        没有匹配的 Issues
      </div>
    )
  }

  return (
    <div className={cn("divide-y", className)}>
      {roots.map((issue) => (
        <TreeNode
          key={issue.id}
          issue={issue}
          depth={0}
          childrenMap={childrenMap}
          issueMap={issueMap}
          relations={relations}
          agentMap={agentMap}
        />
      ))}
    </div>
  )
}

interface TreeNodeProps {
  issue: Issue
  depth: number
  childrenMap: Map<string, Issue[]>
  issueMap: Map<string, Issue>
  relations: IssueRelation[]
  agentMap: Map<string, string>
}

function TreeNode({
  issue,
  depth,
  childrenMap,
  issueMap,
  relations,
  agentMap,
}: TreeNodeProps) {
  const children = childrenMap.get(issue.id) ?? []
  const [open, setOpen] = React.useState(depth < 2)
  const completed = children.filter((child) =>
    terminalStatuses.has(child.status)
  ).length
  const blockers = relations
    .filter((relation) => relation.relatedIssueId === issue.id)
    .flatMap((relation) => {
      const blocker = issueMap.get(relation.issueId)
      return blocker && !terminalStatuses.has(blocker.status) ? [blocker] : []
    })
  const progress = children.length
    ? Math.round((completed / children.length) * 100)
    : 0

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div
        className={cn(
          "group flex min-w-0 items-start gap-2 px-4 py-3 transition-colors hover:bg-muted/35",
          depth > 0 && "border-l border-border/70"
        )}
        style={{ marginLeft: Math.min(depth, 4) * 20 }}
      >
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center">
          {children.length ? (
            <CollapsibleTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={open ? "折叠子 Issues" : "展开子 Issues"}
                />
              }
            >
              <ChevronDown
                className={cn("transition-transform", !open && "-rotate-90")}
              />
            </CollapsibleTrigger>
          ) : (
            <CircleDotDashed className="size-3.5 text-muted-foreground/55" />
          )}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <Link
              to={`/issues/${issue.id}`}
              className="truncate text-sm font-medium hover:underline"
            >
              {issue.title}
            </Link>
            <span className="font-mono text-[11px] text-muted-foreground">
              {issue.identifier}
            </span>
            <StatusBadge
              status={issue.status}
              className="h-5 px-1.5 text-[10px]"
            />
            {issue.executionPhase === "waiting_children" && (
              <Badge
                variant="secondary"
                className="h-5 gap-1 px-1.5 text-[10px]"
              >
                <GitBranch className="size-3" />
                等待子树
              </Badge>
            )}
            {issue.executionPhase === "resuming" && (
              <Badge
                variant="secondary"
                className="h-5 gap-1 px-1.5 text-[10px]"
              >
                <RotateCcw className="size-3" />
                汇总中
              </Badge>
            )}
          </div>

          <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
            <span>
              {agentMap.get(issue.assigneeAgentId ?? "") ?? "未分配 Agent"}
            </span>
            <span>深度 {issue.requestDepth}</span>
            {children.length > 0 && (
              <span>
                {completed}/{children.length} 个直属子项完成
              </span>
            )}
            {blockers.length > 0 && (
              <Tooltip>
                <TooltipTrigger className="inline-flex items-center gap-1 text-amber-700 dark:text-amber-300">
                  <LockKeyhole className="size-3" />
                  {blockers.length} 个未完成依赖
                </TooltipTrigger>
                <TooltipContent>
                  {blockers.map((blocker) => blocker.identifier).join("、")}
                </TooltipContent>
              </Tooltip>
            )}
          </div>
          {children.length > 0 && (
            <Progress
              value={progress}
              className="mt-2 max-w-xs"
              aria-label="子 Issue 完成进度"
            />
          )}
        </div>

        <Badge
          variant="outline"
          className="mt-0.5 hidden shrink-0 gap-1 sm:flex"
        >
          <GitBranch className="size-3" />
          {children.length}
        </Badge>
      </div>

      {children.length > 0 && (
        <CollapsibleContent>
          <div className="border-t border-border/45 bg-muted/10">
            {children.map((child) => (
              <TreeNode
                key={child.id}
                issue={child}
                depth={depth + 1}
                childrenMap={childrenMap}
                issueMap={issueMap}
                relations={relations}
                agentMap={agentMap}
              />
            ))}
          </div>
        </CollapsibleContent>
      )}
    </Collapsible>
  )
}
