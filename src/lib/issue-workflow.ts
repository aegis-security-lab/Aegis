import type { IssueLabel, IssueStatus } from "@/types"

export const workflowStatuses: IssueStatus[] = [
  "todo",
  "in_progress",
  "in_review",
  "done",
  "cancelled",
]

export const workflowStatusLabels: Record<IssueStatus, string> = {
  todo: "待执行",
  in_progress: "处理中",
  in_review: "待复核",
  done: "已完成",
  cancelled: "已取消",
}

// Workflow status is the five-state business projection. Older data may still
// carry legacy statuses until the backend startup migration rewrites them, so
// map them defensively onto the five-state vocabulary.
export function issueWorkflowStatus(status: IssueStatus | string): IssueStatus {
  if (status === "backlog") return "todo"
  if (status === "blocked") return "in_progress"
  if (status === "failed" || status === "budget_exceeded") return "done"
  if (!workflowStatuses.includes(status as IssueStatus)) return "todo"
  return status as IssueStatus
}

export const issueLabelMeta: Record<
  IssueLabel,
  { label: string; className: string }
> = {
  blocked: {
    label: "阻塞",
    className: "border-destructive/25 bg-destructive/10 text-destructive",
  },
  failed: {
    label: "失败",
    className: "border-destructive/25 bg-destructive/10 text-destructive",
  },
  budget_exceeded: {
    label: "超出预算",
    className: "border-warning/25 bg-warning/10 text-warning",
  },
}

export const issueLabelOptions: IssueLabel[] = [
  "blocked",
  "failed",
  "budget_exceeded",
]

export function issueLabelsOf(
  issue: { labels?: string[] } | undefined
): IssueLabel[] {
  return (issue?.labels ?? []).filter(
    (label): label is IssueLabel => label in issueLabelMeta
  )
}

export function hasIssueLabel(
  issue: { labels?: string[] } | undefined,
  ...labels: IssueLabel[]
) {
  const present = new Set(issue?.labels ?? [])
  return labels.some((label) => present.has(label))
}
