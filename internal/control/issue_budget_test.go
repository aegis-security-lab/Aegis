package control

import (
	"strings"
	"testing"
	"time"
)

func TestIssueBudgetDefaultsAndCanBeFullyDisabled(t *testing.T) {
	budget := normalizeIssueBudget(IssueBudgetConfig{})
	if budget.TimeLimitMinutes == nil || *budget.TimeLimitMinutes != 10 || budget.CheckIntervalSeconds != 60 || budget.TokenLimit != nil || budget.CostLimit != nil {
		t.Fatalf("unexpected default budget: %+v", budget)
	}
	disabled := normalizeIssueBudget(IssueBudgetConfig{CheckIntervalSeconds: 60})
	if disabled.TokenLimit != nil || disabled.CostLimit != nil || disabled.TimeLimitMinutes != nil {
		t.Fatalf("explicitly disabled budget was re-enabled: %+v", disabled)
	}
}

func TestIssueBudgetReasonReportsEveryReachedLimit(t *testing.T) {
	tokens := int64(1000)
	cost := 2.5
	minutes := 10
	reason := issueBudgetExceededReason(IssueBudgetConfig{TokenLimit: &tokens, CostLimit: &cost, TimeLimitMinutes: &minutes, CheckIntervalSeconds: 60}, issueBudgetUsage{Tokens: 1200, Cost: 3, Duration: 11 * time.Minute})
	for _, expected := range []string{"1200 tokens", "$3.000000", "11.0 分钟"} {
		if !strings.Contains(reason, expected) {
			t.Fatalf("budget reason missing %q: %s", expected, reason)
		}
	}
}

func TestBudgetExhaustionAbandonsTopLevelIssueWithReason(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Budgeted task", Objective: "Complete within budget", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	execution, _ := store.createExecution(issue, "backend-engineer", "work")
	now := time.Now()
	started := now.Add(-11 * time.Minute)
	_ = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{"status": "running", "started_at": started}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID}).Error
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.checkIssueBudgets(now)

	finished, _ := store.GetIssue(issue.ID)
	if finished.Status != "cancelled" || !finished.ObjectiveAbandoned || finished.AbandonmentReason == "" || !strings.Contains(finished.AbandonmentReason, "时间预算已达到") {
		t.Fatalf("budget exhaustion did not abandon with a reason: %+v", finished)
	}
	if !strings.Contains(finished.Result, "未生成额外总结") {
		t.Fatalf("missing fallback summary after budget abandonment: %q", finished.Result)
	}
}
