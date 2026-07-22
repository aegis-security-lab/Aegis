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

func TestTaskTimeBudgetOverridesGlobalBudget(t *testing.T) {
	store := configuredStore(t)
	minutes := 2
	task, issue, err := store.CreateTask(CreateIssueInput{
		Title: "Custom budget task", Objective: "Finish quickly", Priority: "high", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", TimeBudgetMinutes: &minutes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.TimeBudgetMinutes == nil || *task.TimeBudgetMinutes != minutes || issue.TimeBudgetMinutes == nil || *issue.TimeBudgetMinutes != minutes {
		t.Fatalf("task budget was not persisted: task=%+v issue=%+v", task, issue)
	}
	budget := budgetForIssue(store.Config(), issue)
	if budget.TimeLimitMinutes == nil || *budget.TimeLimitMinutes != minutes {
		t.Fatalf("task budget did not override global budget: %+v", budget)
	}
}

func TestTaskTimeBudgetRejectsNonPositiveValue(t *testing.T) {
	store := configuredStore(t)
	minutes := 0
	if _, _, err := store.CreateTask(CreateIssueInput{Title: "Invalid budget", Priority: "medium", WorkMode: "autonomous", TimeBudgetMinutes: &minutes}); err == nil {
		t.Fatal("non-positive task budget was accepted")
	}
}

func TestBudgetExhaustionAbandonsTopLevelIssueWithReason(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Budgeted task", Objective: "Complete within budget", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	execution, _ := store.createExecution(issue, "backend-engineer", "work")
	now := time.Now()
	started := now.Add(-11 * time.Minute)
	_ = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{"status": "running", "started_at": started}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID, "started_at": started}).Error
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

func TestBudgetExhaustionDoesNotApplyToChildIssue(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root task", Objective: "Complete root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Long child", Objective: "Complete child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, _ := store.createExecution(child, "frontend-engineer", "work")
	now := time.Now()
	_ = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{"status": "running", "started_at": now.Add(-30 * time.Minute)}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID}).Error

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.checkIssueBudgets(now)

	unchanged, _ := store.GetIssue(child.ID)
	if unchanged.Status != "in_progress" || unchanged.ObjectiveAbandoned || unchanged.AbandonRequestedAt != nil {
		t.Fatalf("child Issue was incorrectly subjected to root budget: %+v", unchanged)
	}
}

func TestIssueBudgetUsesOnlyCurrentExecutionAndResetsOnRestart(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Restarted root", Objective: "Complete the current request", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	previous, _ := store.createExecution(root, "backend-engineer", "work")
	current, _ := store.createExecution(root, "backend-engineer", "wakeup")
	now := time.Now()
	_ = store.db.Model(&Execution{}).Where("id = ?", previous.ID).Updates(map[string]any{
		"status": "completed", "started_at": now.Add(-45 * time.Minute), "finished_at": now.Add(-15 * time.Minute),
		"tokens": int64(9000), "cost": 9.0,
	}).Error
	_ = store.db.Model(&Execution{}).Where("id = ?", current.ID).Updates(map[string]any{
		"status": "running", "started_at": now.Add(-6 * time.Minute), "tokens": int64(100), "cost": 0.25,
	}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", root.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": current.ID, "current_execution_id": current.ID,
	}).Error

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	usage, ok := manager.issueBudgetUsage(root.ID, now)
	if !ok {
		t.Fatal("root budget usage was not available")
	}
	if usage.Duration != 6*time.Minute {
		t.Fatalf("restarted execution used %s of time budget, want 6m", usage.Duration)
	}
	if usage.Tokens != 100 || usage.Cost != 0.25 {
		t.Fatalf("previous execution usage leaked into the restarted execution: %+v", usage)
	}

	manager.checkIssueBudgets(now)
	unchanged, _ := store.GetIssue(root.ID)
	if unchanged.Status != "in_progress" || unchanged.ObjectiveAbandoned {
		t.Fatalf("previous execution exhausted the restarted execution budget: %+v", unchanged)
	}
}

func TestIssueBudgetDoesNotRunWithoutAnActiveExecution(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Waiting root", Objective: "Complete tree", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	execution, _ := store.createExecution(root, "backend-engineer", "work")
	now := time.Now()
	_ = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{
		"status": "completed", "started_at": now.Add(-30 * time.Minute), "finished_at": now.Add(-20 * time.Minute),
	}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", root.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "waiting_children", "checkout_execution_id": "", "current_execution_id": execution.ID,
	}).Error

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.checkIssueBudgets(now)

	unchanged, _ := store.GetIssue(root.ID)
	if unchanged.Status != "in_progress" || unchanged.ObjectiveAbandoned {
		t.Fatalf("completed execution kept spending Issue budget: %+v", unchanged)
	}
}
