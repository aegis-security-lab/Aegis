import { toast } from "sonner"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { updateIssue } from "@/lib/api"
import {
  issueWorkflowStatus,
  workflowStatuses,
  workflowStatusLabels,
} from "@/lib/issue-workflow"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, IssueStatus } from "@/types"

const statusDot: Record<IssueStatus, string> = {
  todo: "bg-info",
  in_progress: "bg-success",
  in_review: "bg-warning",
  done: "bg-muted-foreground",
  cancelled: "bg-muted-foreground/40",
}

export function IssueStatusSelect({
  issue,
  className,
  size = "sm",
  align = "center",
  ariaLabel = "修改 Issue 状态",
}: {
  issue: Issue
  className?: string
  size?: "sm" | "default"
  align?: "start" | "center" | "end"
  ariaLabel?: string
}) {
  const { setState, refresh } = useAppState()
  const value = issueWorkflowStatus(issue.status)

  const change = async (status: IssueStatus) => {
    if (status === value) return
    const previous = issue.status
    setState((current) =>
      current
        ? {
            ...current,
            issues: current.issues.map((candidate) =>
              candidate.id === issue.id ? { ...candidate, status } : candidate
            ),
          }
        : current
    )
    try {
      await updateIssue(issue.id, { status })
      toast.success(`已移动到「${workflowStatusLabels[status]}」`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "修改状态失败")
      setState((current) =>
        current
          ? {
              ...current,
              issues: current.issues.map((candidate) =>
                candidate.id === issue.id
                  ? { ...candidate, status: previous }
                  : candidate
              ),
            }
          : current
      )
      void refresh()
    }
  }

  return (
    <Select
      value={value}
      onValueChange={(next) => {
        if (next) void change(next as IssueStatus)
      }}
    >
      <SelectTrigger
        size={size}
        aria-label={ariaLabel}
        className={cn(
          "h-5 gap-1 rounded-md px-1.5 py-0 text-[10px] font-medium",
          className
        )}
      >
        <span className={cn("size-1.5 shrink-0 rounded-full", statusDot[value])} />
        <SelectValue />
      </SelectTrigger>
      <SelectContent align={align} className="min-w-36">
        <SelectGroup>
          {workflowStatuses.map((status) => (
            <SelectItem key={status} value={status}>
              <span
                className={cn(
                  "size-1.5 shrink-0 rounded-full",
                  statusDot[status]
                )}
              />
              {workflowStatusLabels[status]}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
