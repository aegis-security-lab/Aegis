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

const statusText: Record<IssueStatus, string> = {
  todo: "text-info",
  in_progress: "text-success",
  in_review: "text-warning",
  done: "text-muted-foreground",
  cancelled: "text-muted-foreground/70",
}

export function IssueStatusSelect({
  issue,
  className,
  align = "start",
  ariaLabel = "修改 Issue 状态",
}: {
  issue: Issue
  className?: string
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
        aria-label={ariaLabel}
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        className={cn(
          "gap-1 rounded-full border-0 bg-transparent px-2 py-0 text-[11px] font-medium shadow-none hover:bg-muted data-open:bg-muted data-open:ring-2 data-open:ring-ring/40",
          statusText[value],
          className
        )}
      >
        <span className={cn("size-1.5 shrink-0 rounded-full", statusDot[value])} />
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        align={align}
        side="bottom"
        sideOffset={4}
        alignItemWithTrigger={false}
        className="min-w-36 p-1"
      >
        <SelectGroup>
          {workflowStatuses.map((status) => (
            <SelectItem key={status} value={status} className="gap-2 pr-7">
              <span
                className={cn(
                  "size-1.5 shrink-0 rounded-full",
                  statusDot[status]
                )}
              />
              <span className={cn("text-xs", statusText[status])}>
                {workflowStatusLabels[status]}
              </span>
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}
