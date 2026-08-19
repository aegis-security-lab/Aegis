package control

import (
	"fmt"
	"strings"
)

const (
	issueRuntimeKindRunning   = "running"
	issueRuntimeKindWaiting   = "waiting"
	issueRuntimeKindFailed    = "failed"
	issueRuntimeKindCompleted = "completed"
	issueRuntimeKindCancelled = "cancelled"
	issueRuntimeKindPending   = "pending"
)

type issueRuntimeFacts struct {
	currentExecution *Execution
	hasActiveWait    bool
	hasValidation    bool
}

func (s *Store) issueRuntimeViews(issues []Issue) []IssueRuntimeView {
	views := make([]IssueRuntimeView, 0, len(issues))
	if len(issues) == 0 {
		return views
	}

	issueIDs := make([]string, 0, len(issues))
	executionIDs := make([]string, 0, len(issues)*2)
	for _, issue := range issues {
		issueIDs = append(issueIDs, issue.ID)
		for _, id := range []string{issue.CurrentExecutionID, issue.RecoveryExecutionID, issue.ValidationExecutionID} {
			if strings.TrimSpace(id) != "" {
				executionIDs = append(executionIDs, id)
			}
		}
	}

	executionByID := make(map[string]Execution, len(executionIDs))
	if len(executionIDs) > 0 {
		var executions []Execution
		_ = s.db.Where("id IN ?", uniqueStrings(executionIDs)).Find(&executions).Error
		for _, execution := range executions {
			executionByID[execution.ID] = execution
		}
	}

	waitingByIssue := make(map[string]bool)
	var waits []IssueChildWait
	_ = s.db.Where("parent_issue_id IN ? AND status = ?", issueIDs, "waiting").Find(&waits).Error
	for _, wait := range waits {
		waitingByIssue[wait.ParentIssueID] = true
	}

	validationByIssue := make(map[string]bool)
	var validations []IssueValidation
	_ = s.db.Where("issue_id IN ? AND status = ?", issueIDs, "running").Find(&validations).Error
	for _, validation := range validations {
		validationByIssue[validation.IssueID] = true
	}

	for _, issue := range issues {
		var current *Execution
		if execution, ok := executionByID[issue.CurrentExecutionID]; ok && execution.IssueID == issue.ID {
			copy := execution
			current = &copy
		}
		views = append(views, resolveIssueRuntimeView(issue, issueRuntimeFacts{
			currentExecution: current,
			hasActiveWait:    waitingByIssue[issue.ID],
			hasValidation:    validationByIssue[issue.ID],
		}))
	}
	return views
}

func resolveIssueRuntimeView(issue Issue, facts issueRuntimeFacts) IssueRuntimeView {
	view := IssueRuntimeView{
		IssueID:            issue.ID,
		State:              "pending",
		Kind:               issueRuntimeKindPending,
		Health:             "healthy",
		CurrentExecutionID: issue.CurrentExecutionID,
		UpdatedAt:          issue.UpdatedAt,
	}
	if facts.currentExecution != nil {
		view.CurrentExecutionStatus = facts.currentExecution.Status
	}

	set := func(state, kind, health, detail string) IssueRuntimeView {
		view.State, view.Kind, view.Health, view.Detail = state, kind, health, detail
		return view
	}

	switch issue.Status {
	case "done":
		if hasIssueLabel(issue, issueLabelBudgetExceeded) {
			return set("budget_exceeded", issueRuntimeKindFailed, "error", issue.Error)
		}
		if hasIssueLabel(issue, issueLabelFailed) {
			return set("failed", issueRuntimeKindFailed, "error", firstNonEmpty(issue.Error, executionError(facts.currentExecution)))
		}
		return set("completed", issueRuntimeKindCompleted, "healthy", "")
	case "cancelled":
		if issue.ObjectiveAbandoned {
			return set("abandoned", issueRuntimeKindCancelled, "healthy", issue.AbandonmentReason)
		}
		return set("cancelled", issueRuntimeKindCancelled, "healthy", issue.AbandonmentReason)
	case "in_progress":
		if hasIssueLabel(issue, issueLabelBlocked) {
			return set("blocked", issueRuntimeKindFailed, "error", firstNonEmpty(issue.Error, executionError(facts.currentExecution)))
		}
	}

	if issue.ExecutionPhase == "recovering" {
		if strings.TrimSpace(issue.RecoveryExecutionID) == "" {
			return set("data_inconsistent", issueRuntimeKindFailed, "error", "Issue 标记为恢复中，但没有 recoveryExecutionId")
		}
		if facts.currentExecution == nil || facts.currentExecution.ID != issue.RecoveryExecutionID {
			return set("data_inconsistent", issueRuntimeKindFailed, "error", "恢复 Execution 不存在，或不是 Issue 当前执行")
		}
		switch facts.currentExecution.Status {
		case "queued", "starting", "running", "waiting_approval":
			return set("recovering", issueRuntimeKindRunning, "healthy", executionDetail(facts.currentExecution))
		case "failed", "cancelled":
			return set("recovery_stalled", issueRuntimeKindFailed, "stalled", firstNonEmpty(facts.currentExecution.Error, "恢复 Execution 已终止"))
		default:
			// A disconnected/completed interrupted execution is the input to the
			// recovery scheduler, not proof that a recovery worker is running.
			return set("recovery_pending", issueRuntimeKindWaiting, "waiting", "等待恢复调度器接管")
		}
	}

	switch issue.ExecutionPhase {
	case "waiting_children":
		if !facts.hasActiveWait {
			return set("wait_stalled", issueRuntimeKindFailed, "stalled", "Issue 标记为等待子任务，但没有有效等待记录")
		}
		return set("waiting_children", issueRuntimeKindWaiting, "waiting", "等待直属子 Issue 完成")
	case "sleeping":
		if strings.TrimSpace(issue.SleepToken) == "" {
			return set("sleep_stalled", issueRuntimeKindFailed, "stalled", "Issue 标记为休眠，但没有有效唤醒令牌")
		}
		return set("sleeping", issueRuntimeKindWaiting, "waiting", "当前 Agent 回合已释放，等待唤醒")
	case "validating":
		if !facts.hasValidation {
			return set("validation_stalled", issueRuntimeKindFailed, "stalled", "Issue 标记为验收中，但没有运行中的验收记录")
		}
	case "scheduled":
		return set("scheduled", issueRuntimeKindPending, "waiting", "等待调度器创建 Execution")
	}

	if facts.currentExecution != nil {
		switch facts.currentExecution.Status {
		case "queued":
			return set("queued", issueRuntimeKindRunning, "healthy", "Execution 正在排队")
		case "starting":
			return set("starting", issueRuntimeKindRunning, "healthy", "Agent 正在启动")
		case "running":
			state := "running"
			if issue.ExecutionPhase == "validating" {
				state = "validating"
			} else if issue.ExecutionPhase == "resuming" {
				state = "resuming"
			} else if issue.ExecutionPhase == "summarizing" {
				state = "summarizing"
			} else if issue.ExecutionPhase == "budget_summarizing" {
				state = "budget_summarizing"
			}
			return set(state, issueRuntimeKindRunning, "healthy", executionDetail(facts.currentExecution))
		case "waiting_approval":
			return set("waiting_approval", issueRuntimeKindWaiting, "waiting", "等待操作员审批工具调用")
		case "failed", "disconnected", "stopped":
			return set("execution_"+facts.currentExecution.Status, issueRuntimeKindFailed, "stalled", firstNonEmpty(facts.currentExecution.Error, fmt.Sprintf("当前 Execution 状态为 %s", facts.currentExecution.Status)))
		}
	}

	if issue.Status == "in_progress" {
		return set("execution_missing", issueRuntimeKindFailed, "stalled", "Issue 正在处理中，但没有活动的当前 Execution")
	}
	if issue.Status == "in_review" {
		return set("in_review", issueRuntimeKindWaiting, "waiting", "等待人工复核")
	}
	return view
}

func executionDetail(execution *Execution) string {
	if execution == nil {
		return ""
	}
	if strings.TrimSpace(execution.CurrentTool) != "" {
		return "正在调用 " + execution.CurrentTool
	}
	return ""
}

func executionError(execution *Execution) string {
	if execution == nil {
		return ""
	}
	return execution.Error
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
