import * as React from "react"
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react"
import {
  ArrowRight,
  CircleDotDashed,
  ExternalLink,
  LockKeyhole,
  Workflow,
} from "lucide-react"
import { useNavigate } from "react-router-dom"

import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { cn } from "@/lib/utils"
import type { AgentDefinition, Execution, Issue, IssueRelation } from "@/types"

const nodeWidth = 288
const nodeHeight = 116
const horizontalGap = 96
const verticalGap = 34
const terminalStatuses = new Set(["done", "cancelled"])
const activeExecutionStatuses = new Set([
  "queued",
  "starting",
  "running",
  "waiting_approval",
])

type DependencyNodeData = {
  issue: Issue
  agentName: string
  incoming: number
  unresolved: number
  outgoing: number
  execution?: Execution
  onOpen: (issueID: string) => void
} & Record<string, unknown>

type DependencyNode = Node<DependencyNodeData, "issue">

interface IssueDependencyFlowProps {
  issues: Issue[]
  relations: IssueRelation[]
  agents: AgentDefinition[]
  executions?: Execution[]
  className?: string
}

const nodeTypes = { issue: DependencyIssueNode }

export function IssueDependencyFlow({
  issues,
  relations,
  agents,
  executions = [],
  className,
}: IssueDependencyFlowProps) {
  const navigate = useNavigate()
  const openIssue = React.useCallback(
    (issueID: string) => navigate(`/issues/${issueID}`),
    [navigate]
  )
  const graph = React.useMemo(
    () => buildDependencyGraph(issues, relations, agents, executions, openIssue),
    [agents, executions, issues, openIssue, relations]
  )
  const statusByIssueID = React.useMemo(
    () => new Map(issues.map((issue) => [issue.id, issue.status])),
    [issues]
  )

  if (issues.length === 0) {
    return (
      <Empty className="min-h-[480px]">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Workflow />
          </EmptyMedia>
          <EmptyTitle>没有匹配的依赖节点</EmptyTitle>
          <EmptyDescription>调整搜索或状态筛选后重试。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <div className={cn("min-w-0", className)}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">
            <CircleDotDashed data-icon="inline-start" />
            {graph.nodes.length} 个 Issue
          </Badge>
          <Badge variant="outline">
            <Workflow data-icon="inline-start" />
            {graph.edges.length} 条依赖
          </Badge>
          {graph.unresolvedEdges > 0 && (
            <Badge variant="outline">
              <LockKeyhole data-icon="inline-start" />
              {graph.unresolvedEdges} 条未解除
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>前置 Issue</span>
          <ArrowRight />
          <span>被阻塞 Issue</span>
        </div>
      </div>
      <div className="h-[min(720px,calc(100dvh-250px))] min-h-[480px] bg-card">
        <ReactFlow<DependencyNode, Edge>
          nodes={graph.nodes}
          edges={graph.edges}
          nodeTypes={nodeTypes}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable
          fitView
          fitViewOptions={{ padding: 0.16, minZoom: 0.35, maxZoom: 1 }}
          minZoom={0.2}
          maxZoom={1.6}
          onlyRenderVisibleElements
          defaultEdgeOptions={{ type: "smoothstep" }}
          onNodeDoubleClick={(_, node) => openIssue(node.id)}
          aria-label="Issue 依赖关系图"
        >
          <Background
            variant={BackgroundVariant.Dots}
            gap={20}
            size={1.2}
            color="var(--border)"
          />
          <Controls showInteractive={false} position="bottom-left" />
          {graph.nodes.length > 8 && (
            <MiniMap
              position="bottom-right"
              pannable
              zoomable
              nodeColor={(node) =>
                dependencyNodeColor(statusByIssueID.get(node.id) ?? "todo")
              }
              nodeStrokeWidth={2}
            />
          )}
        </ReactFlow>
      </div>
    </div>
  )
}

function DependencyIssueNode({ data, selected }: NodeProps<DependencyNode>) {
  const running =
    data.issue.status === "in_progress" ||
    (data.execution && activeExecutionStatuses.has(data.execution.status))
  const detail = dependencyNodeDetail(data.issue, data.execution, data.unresolved)

  return (
    <div
      className={cn(
        "relative w-72 rounded-xl border bg-card p-3 text-card-foreground shadow-sm transition-[border-color,box-shadow]",
        selected && "border-primary shadow-md"
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!size-2.5 !border-2 !border-background !bg-muted-foreground"
      />
      <Handle
        type="source"
        position={Position.Right}
        className="!size-2.5 !border-2 !border-background !bg-primary"
      />

      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-[11px] text-muted-foreground">
              {data.issue.identifier}
            </span>
            <StatusBadge status={data.issue.status} className="h-5" />
          </div>
          <p className="mt-1.5 line-clamp-2 text-sm font-medium leading-5" title={data.issue.title}>
            {data.issue.title}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="nodrag shrink-0"
          onClick={() => data.onOpen(data.issue.id)}
          aria-label={`打开 ${data.issue.identifier}`}
        >
          <ExternalLink />
        </Button>
      </div>

      <div className="mt-2 flex min-w-0 items-center justify-between gap-2 text-[11px] text-muted-foreground">
        <span className="truncate">{data.agentName}</span>
        <span className="shrink-0">
          {data.incoming} 入 · {data.outgoing} 出
        </span>
      </div>
      <div
        className={cn(
          "mt-2 truncate rounded-md bg-muted px-2 py-1 text-[11px] text-muted-foreground",
          data.unresolved > 0 && "text-foreground"
        )}
        title={detail}
      >
        {running && <span className="mr-1 inline-block size-1.5 rounded-full bg-primary" />}
        {detail}
      </div>
    </div>
  )
}

function buildDependencyGraph(
  issues: Issue[],
  relations: IssueRelation[],
  agents: AgentDefinition[],
  executions: Execution[],
  onOpen: (issueID: string) => void
) {
  const issueMap = new Map(issues.map((issue) => [issue.id, issue]))
  const agentMap = new Map(agents.map((agent) => [agent.id, agent.name]))
  const executionMap = newestExecutionByIssue(executions)
  const relevantRelations = relations.filter(
    (relation) =>
      relation.type === "blocks" &&
      issueMap.has(relation.issueId) &&
      issueMap.has(relation.relatedIssueId)
  )
  const outgoing = new Map<string, string[]>()
  const incoming = new Map<string, string[]>()
  for (const issue of issues) {
    outgoing.set(issue.id, [])
    incoming.set(issue.id, [])
  }
  for (const relation of relevantRelations) {
    outgoing.get(relation.issueId)?.push(relation.relatedIssueId)
    incoming.get(relation.relatedIssueId)?.push(relation.issueId)
  }
  const positions = topologicalLayout(issues, outgoing, incoming)
  const nodes: DependencyNode[] = issues.map((issue) => {
    const dependencies = incoming.get(issue.id) ?? []
    return {
      id: issue.id,
      type: "issue",
      position: positions.get(issue.id) ?? { x: 0, y: 0 },
      data: {
        issue,
        agentName:
          agentMap.get(issue.assigneeAgentId ?? "") ?? "未分配 Agent",
        incoming: dependencies.length,
        unresolved: dependencies.filter((id) => {
          const blocker = issueMap.get(id)
          return blocker && !terminalStatuses.has(blocker.status)
        }).length,
        outgoing: outgoing.get(issue.id)?.length ?? 0,
        execution: executionMap.get(issue.id),
        onOpen,
      },
    }
  })
  let unresolvedEdges = 0
  const edges: Edge[] = relevantRelations.map((relation) => {
    const source = issueMap.get(relation.issueId)!
    const unresolved = !terminalStatuses.has(source.status)
    if (unresolved) unresolvedEdges++
    return {
      id: relation.id,
      source: relation.issueId,
      target: relation.relatedIssueId,
      type: "smoothstep",
      animated: unresolved,
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: unresolved ? "var(--primary)" : "var(--muted-foreground)",
      },
      style: {
        stroke: unresolved ? "var(--primary)" : "var(--muted-foreground)",
        strokeWidth: unresolved ? 2 : 1.35,
      },
    }
  })
  return { nodes, edges, unresolvedEdges }
}

function topologicalLayout(
  issues: Issue[],
  outgoing: Map<string, string[]>,
  incoming: Map<string, string[]>
) {
  const indegree = new Map(
    issues.map((issue) => [issue.id, incoming.get(issue.id)?.length ?? 0])
  )
  const rank = new Map(issues.map((issue) => [issue.id, 0]))
  const issueMap = new Map(issues.map((issue) => [issue.id, issue]))
  const queue = issues
    .filter((issue) => indegree.get(issue.id) === 0)
    .sort((a, b) => a.number - b.number)
    .map((issue) => issue.id)
  const visited = new Set<string>()

  for (let index = 0; index < queue.length; index++) {
    const issueID = queue[index]
    visited.add(issueID)
    for (const targetID of outgoing.get(issueID) ?? []) {
      rank.set(targetID, Math.max(rank.get(targetID) ?? 0, (rank.get(issueID) ?? 0) + 1))
      const nextIndegree = (indegree.get(targetID) ?? 1) - 1
      indegree.set(targetID, nextIndegree)
      if (nextIndegree === 0) queue.push(targetID)
    }
  }
  for (const issue of issues) {
    if (!visited.has(issue.id)) rank.set(issue.id, 0)
  }

  const columns = new Map<number, Issue[]>()
  for (const issue of issues) {
    const column = rank.get(issue.id) ?? 0
    const items = columns.get(column) ?? []
    items.push(issue)
    columns.set(column, items)
  }
  for (const items of columns.values()) {
    items.sort((a, b) => {
      const aParents = incoming.get(a.id)?.map((id) => issueMap.get(id)?.number ?? 0) ?? []
      const bParents = incoming.get(b.id)?.map((id) => issueMap.get(id)?.number ?? 0) ?? []
      const aAnchor = aParents.length ? Math.min(...aParents) : a.number
      const bAnchor = bParents.length ? Math.min(...bParents) : b.number
      return aAnchor - bAnchor || a.number - b.number
    })
  }
  const maxRows = Math.max(1, ...[...columns.values()].map((items) => items.length))
  const positions = new Map<string, { x: number; y: number }>()
  for (const [column, items] of columns) {
    const offset = ((maxRows - items.length) * (nodeHeight + verticalGap)) / 2
    items.forEach((issue, row) => {
      positions.set(issue.id, {
        x: column * (nodeWidth + horizontalGap),
        y: offset + row * (nodeHeight + verticalGap),
      })
    })
  }
  return positions
}

function newestExecutionByIssue(executions: Execution[]) {
  const result = new Map<string, Execution>()
  for (const execution of executions) {
    const current = result.get(execution.issueId)
    if (!current || execution.startedAt > current.startedAt) {
      result.set(execution.issueId, execution)
    }
  }
  return result
}

function dependencyNodeDetail(
  issue: Issue,
  execution: Execution | undefined,
  unresolved: number
) {
  if (issue.error) return issue.error
  if (issue.executionPhase === "waiting_children") return "等待直属子 Issue 完成"
  if (issue.executionPhase === "validating") return "验收 Agent 正在核验目标"
  if (issue.executionPhase === "resuming") return "正在汇总子树结果"
  if (execution?.currentTool) return `正在调用 ${execution.currentTool}`
  if (execution && activeExecutionStatuses.has(execution.status)) {
    return `Execution ${execution.status}`
  }
  if (unresolved > 0) return `等待 ${unresolved} 个前置依赖`
  if (issue.status === "done") return "依赖已经解除"
  if (issue.status === "cancelled") return "Issue 已取消"
  return "等待调度"
}

function dependencyNodeColor(status: string) {
  if (status === "done") return "var(--muted-foreground)"
  if (status === "cancelled" || status === "blocked") return "var(--destructive)"
  if (status === "in_progress") return "var(--primary)"
  return "var(--border)"
}
