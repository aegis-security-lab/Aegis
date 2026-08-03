import type { IssueStatus } from "@/types"

// Workflow status is the five-state business projection. Operational outcomes
// such as failure, budget exhaustion and blocking stay visible through the
// runtime view without becoming a second competing workflow state machine.
export function issueWorkflowStatus(status: IssueStatus): IssueStatus {
  return ["backlog", "blocked", "failed", "budget_exceeded"].includes(status)
    ? "todo"
    : status
}
