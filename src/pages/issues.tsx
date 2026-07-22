import * as React from "react"
import { Search } from "lucide-react"

import { IssueTree } from "@/components/issue-tree"
import { PageHeader } from "@/components/page-header"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useAppState } from "@/lib/state"
import type { Issue, IssueStatus } from "@/types"

const statuses: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "blocked",
  "done",
  "cancelled",
]

export function IssuesPage() {
  const { state } = useAppState()
  const [query, setQuery] = React.useState("")
  const [filter, setFilter] = React.useState("all")
  const allIssues = state?.issues ?? []
  const matched = allIssues.filter(
    (issue) =>
      (filter === "all" || issue.status === filter) &&
      `${issue.identifier} ${issue.title}`
        .toLowerCase()
        .includes(query.toLowerCase())
  )
  const hierarchyIssues = includeAncestors(matched, allIssues)
  const displayedIssues = hierarchyIssues

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Shared work"
        title="Issues"
        description="按父子关系展示任务拆解层级与每个 Issue 的执行状态。"
      />

      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative max-w-md flex-1">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索编号或标题…"
          />
        </div>
        <Select
          value={filter}
          onValueChange={(value) => setFilter(String(value))}
        >
          <SelectTrigger className="w-full sm:w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            {statuses.map((status) => (
              <SelectItem key={status} value={status}>
                {status}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div>
        <div className="mb-3 flex flex-wrap items-center justify-end gap-3">
          <span className="text-xs text-muted-foreground">
            {matched.length} / {allIssues.length} Issues
          </span>
        </div>

        <p className="mb-3 text-xs text-muted-foreground">按父子关系展示任务拆解层级与直属子项进度。</p>
        <Card className="overflow-hidden p-0">
          <IssueTree
            mode="hierarchy"
            issues={displayedIssues}
            relations={state?.relations ?? []}
            agents={state?.agents ?? []}
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
