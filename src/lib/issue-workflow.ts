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

export function issueWorkflowStatus(status: IssueStatus): IssueStatus {
  if (!workflowStatuses.includes(status)) return "todo"
  return status
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
