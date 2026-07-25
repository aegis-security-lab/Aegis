import * as React from "react"
import { ChevronsDown, ChevronsUp } from "lucide-react"

import { IssueTree } from "@/components/issue-tree"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
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
  const [filter, setFilter] = React.useState("all")
  const [expansion, setExpansion] = React.useState<"none" | "roots" | "all">("roots")
  const [expansionVersion, setExpansionVersion] = React.useState(0)
  const allIssues = state?.issues ?? []
  const matched = allIssues.filter(
    (issue) =>
      filter === "all" || issue.status === filter
  )
  const hierarchyIssues = includeAncestors(matched, allIssues)
  const displayedIssues = hierarchyIssues

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ToggleGroup value={[filter]} onValueChange={(value) => setFilter(String(value[0] ?? "all"))} variant="outline" size="sm">
          <ToggleGroupItem value="all">全部</ToggleGroupItem>
          {statuses.map((status) => <ToggleGroupItem key={status} value={status}>{status}</ToggleGroupItem>)}
        </ToggleGroup>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => { setExpansion("all"); setExpansionVersion((value) => value + 1) }}><ChevronsDown />展开全部</Button>
          <Button size="sm" variant="outline" onClick={() => { setExpansion("none"); setExpansionVersion((value) => value + 1) }}><ChevronsUp />收起全部</Button>
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
