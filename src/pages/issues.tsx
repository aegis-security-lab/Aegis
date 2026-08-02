import * as React from "react"
import { ChevronsDown, ChevronsUp, Search } from "lucide-react"

import { IssueTree } from "@/components/issue-tree"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useAppState } from "@/lib/state"
import type { Issue, IssueStatus } from "@/types"

const statuses: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "blocked",
  "failed",
  "budget_exceeded",
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

export function IssuesPage() {
  const { state } = useAppState()
  const [filter, setFilter] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [expansion, setExpansion] = React.useState<"none" | "roots" | "all">(
    "roots"
  )
  const [expansionVersion, setExpansionVersion] = React.useState(0)
  const allIssues = state?.issues ?? []
  const matched = allIssues.filter(
    (issue) =>
      (filter === "all" || issue.status === filter) &&
      (!query.trim() ||
        `${issue.identifier} ${issue.title}`
          .toLocaleLowerCase()
          .includes(query.trim().toLocaleLowerCase()))
  )
  const hierarchyIssues = includeAncestors(matched, allIssues)
  const displayedIssues = hierarchyIssues

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        eyebrow="Work / Issues"
        title="计划树"
        description="查看任务拆解、负责人、阻塞关系与每个 Issue 的当前状态。"
      />
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="relative min-w-0 flex-1 lg:max-w-xs">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            aria-label="搜索 Issue"
            placeholder="搜索编号或标题…"
            className="pl-8"
          />
        </div>
        <div className="flex min-w-0 items-center gap-2">
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
        </div>
      </div>

      <div>
        <Card className="overflow-hidden p-0">
          <IssueTree
            key={`${filter}-${expansion}-${expansionVersion}`}
            mode="hierarchy"
            defaultExpansion={expansion}
            issues={displayedIssues}
            relations={state?.relations ?? []}
            agents={state?.agents ?? []}
            taskAgents={state?.taskAgents ?? []}
            executions={state?.executions ?? []}
          />
        </Card>
      </div>
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
