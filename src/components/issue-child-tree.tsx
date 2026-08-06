import * as React from "react"
import { ChevronRight } from "lucide-react"
import { Link } from "react-router-dom"

import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { cn } from "@/lib/utils"
import type { Issue, IssueRuntimeView } from "@/types"

export function IssueChildTree({
  rootId,
  issues,
  runtimes = [],
}: {
  rootId: string
  issues: Issue[]
  runtimes?: IssueRuntimeView[]
}) {
  const [collapsed, setCollapsed] = React.useState<Set<string>>(new Set())
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
  const runtimeMap = React.useMemo(() => issueRuntimeMap(runtimes), [runtimes])
  const root = issueMap.get(rootId)
  if (!root) return null

  const toggle = (issueId: string) => {
    setCollapsed((current) => {
      const next = new Set(current)
      if (next.has(issueId)) next.delete(issueId)
      else next.add(issueId)
      return next
    })
  }

  const renderIssue = (issue: Issue, depth: number): React.ReactNode => {
    const children = childrenMap.get(issue.id) ?? []
    const isOpen = !collapsed.has(issue.id)
    return (
      <div key={issue.id}>
        <div
          className="flex min-h-8 items-center gap-1.5 rounded-md py-0.5 pr-1 hover:bg-muted/40"
          style={{ paddingLeft: depth * 18 + 2 }}
        >
          {children.length > 0 ? (
            <button
              type="button"
              onClick={() => toggle(issue.id)}
              aria-expanded={isOpen}
              aria-label={`${isOpen ? "折叠" : "展开"}子 Issues：${issue.title}`}
              className="flex size-5 shrink-0 items-center justify-center rounded-full hover:bg-muted"
            >
              <ChevronRight
                className={cn(
                  "size-3.5 transition-transform",
                  isOpen && "rotate-90"
                )}
              />
            </button>
          ) : (
            <span className="size-5 shrink-0" />
          )}
          <Link
            to={`/issues/${issue.id}`}
            className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-sm"
          >
            <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
              {issue.identifier}
            </span>
            <span className="min-w-0 flex-1 truncate">{issue.title}</span>
            <IssueRuntimeBadge
              runtime={issueRuntimeOrUnavailable(runtimeMap, issue.id)}
            />
          </Link>
        </div>
        {children.length > 0 && isOpen
          ? children.map((child) => renderIssue(child, depth + 1))
          : null}
      </div>
    )
  }

  return (
    <div className="flex flex-col">
      {(childrenMap.get(root.id) ?? []).map((child) => renderIssue(child, 0))}
    </div>
  )
}
