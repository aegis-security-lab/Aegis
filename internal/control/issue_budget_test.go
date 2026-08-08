package control

import (
	"context"
	"strings"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

func TestIssueBudgetDefaults(t *testing.T) {
	budget := normalizeIssueBudget(IssueBudgetConfig{})
	if budget.MaxTurns != 100 || budget.ActiveTimeMinutes != 20 || budget.SummaryTurns != 10 || budget.SummaryTimeMinutes != 3 {
		t.Fatalf("unexpected default budget: %+v", budget)
	}
	if err := validateIssueBudget(budget); err != nil {
		t.Fatal(err)
	}
}

func TestTaskTimeBudgetIsWallClockAndDoesNotResetWithExecution(t *testing.T) {
	store := configuredStore(t)
	minutes := 30
	_, root, err := store.CreateTask(CreateIssueInput{
		Title: "Wall clock task", Objective: "Finish", Priority: "high", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", TimeBudgetMinutes: &minutes,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := root.CreatedAt.Add(17 * time.Minute)
	if got := taskWallClockUsage(root, now); got != 17*time.Minute {
		t.Fatalf("wall usage=%s", got)
	}
	// A new Execution has no influence on the Task clock anchor.
	if _, err := store.createExecution(root, "backend-engineer", "work"); err != nil {
		t.Fatal(err)
	}
	limit, elapsed, remaining, configured := taskWallClockRemaining(root, now)
	if !configured || limit != 30*time.Minute || elapsed != 17*time.Minute || remaining != 13*time.Minute {
		t.Fatalf("unexpected wall budget: limit=%s elapsed=%s remaining=%s configured=%v", limit, elapsed, remaining, configured)
	}
}

func TestChildExecutionSnapshotsBudgetAndNewExecutionResetsIt(t *testing.T) {
	store := configuredStore(t)
	store.mu.Lock()
	store.config.IssueBudget = IssueBudgetConfig{MaxTurns: 7, ActiveTimeMinutes: 9, SummaryTurns: 2, SummaryTimeMinutes: 1}
	store.mu.Unlock()
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	first, err := store.createExecution(child, "frontend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if first.BudgetMaxTurns != 7 || first.BudgetActiveMinutes != 9 || first.BudgetSummaryTurns != 2 || first.BudgetPhase != "active" {
		t.Fatalf("first snapshot=%+v", first)
	}
	store.mu.Lock()
	store.config.IssueBudget = IssueBudgetConfig{MaxTurns: 100, ActiveTimeMinutes: 20, SummaryTurns: 10, SummaryTimeMinutes: 3}
	store.mu.Unlock()
	second, err := store.createExecution(child, "frontend-engineer", "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	if second.BudgetMaxTurns != 100 || second.BudgetActiveMinutes != 20 || second.BudgetTurnsUsed != 0 || second.BudgetSummaryUsed != 0 {
		t.Fatalf("new Execution did not receive a fresh snapshot: %+v", second)
	}
	if first.BudgetMaxTurns != 7 {
		t.Fatal("the first snapshot was mutated by a settings change")
	}
}

func TestExecutionBudgetTransitionsToRestrictedSummary(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, _ := store.createExecution(child, "frontend-engineer", "work")
	_ = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Updates(map[string]any{"status": "running", "budget_max_turns": 2, "budget_summary_turns": 2}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "in_progress", "current_execution_id": execution.ID, "checkout_execution_id": execution.ID}).Error
	execution.BudgetMaxTurns = 2
	execution.BudgetSummaryTurns = 2
	execution.BudgetActiveMinutes = 20
	execution.BudgetSummaryMinutes = 3
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	controller := newExecutionBudgetController(manager, execution)
	config := agentcore.Config{MaxTurns: 50, SystemPrompt: "base"}
	if err := controller.ConfigureAgent(context.Background(), agenthostSpecForTest(), &config); err != nil {
		t.Fatal(err)
	}
	if config.MaxTurns != 4 {
		t.Fatalf("max turns=%d, want work+summary=4", config.MaxTurns)
	}
	request := &agentcore.ModelRequest{}
	_ = controller.BeforeModelCall(context.Background(), request)
	_ = controller.BeforeModelCall(context.Background(), request)
	turn := agentcore.TurnContext{Turn: 2}
	if err := controller.PrepareNextTurn(context.Background(), &turn); err != nil {
		t.Fatal(err)
	}
	outcome := controller.Outcome()
	if !outcome.Exceeded || !strings.Contains(outcome.Reason, "2 个模型轮次") {
		t.Fatalf("outcome=%+v", outcome)
	}
	current, _ := store.GetIssue(child.ID)
	if current.ExecutionPhase != "budget_summarizing" {
		t.Fatalf("phase=%s", current.ExecutionPhase)
	}
	decision, err := controller.BeforeToolCall(context.Background(), agentcore.ToolCallContext{Turn: 3, Call: agentcore.ToolCall{Name: "phone_board_delegate"}})
	if err != nil || !decision.Block {
		t.Fatalf("summary tool was not blocked: decision=%+v err=%v", decision, err)
	}
	decision, err = controller.BeforeToolCall(context.Background(), agentcore.ToolCallContext{Turn: 3, Call: agentcore.ToolCall{Name: "aegis_submit_budget_summary"}})
	if err != nil || decision.Block {
		t.Fatalf("dedicated budget summary tool was blocked: decision=%+v err=%v", decision, err)
	}
}

func TestBudgetOutcomeIsBusinessStatusAndCanBeReopened(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, _ := store.createExecution(child, "frontend-engineer", "work")
	_ = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "in_progress", "current_execution_id": execution.ID, "checkout_execution_id": execution.ID}).Error
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	runner := NativeIssueRunner{Manager: manager}
	prepared := preparedIssueExecution{issue: child, execution: execution, agent: AgentDefinition{ID: "frontend-engineer"}}
	err := runner.finalizeBudgetExceeded(prepared, "已完成调查，建议继续同一方向。", executionBudgetOutcome{Exceeded: true, Reason: "达到轮数预算", TurnsUsed: 12, SummaryUsed: 2, ExceededAt: time.Now()}, map[string]any{"finished_at": time.Now()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	finished, _ := store.GetIssue(child.ID)
	if finished.Status != "done" || !hasIssueLabel(finished, issueLabelBudgetExceeded) || !issueStatusTerminal(finished.Status) || finished.Result == "" {
		t.Fatalf("unexpected budget outcome: %+v", finished)
	}
	todo := "todo"
	reopened, err := store.UpdateIssue(child.ID, UpdateIssueInput{Status: &todo})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "todo" || reopened.ExecutionPhase != "active" || reopened.CompletedAt != nil {
		t.Fatalf("budget Issue was not reopenable: %+v", reopened)
	}
}

func TestParentCanExplicitlyContinueFailedOrBudgetExceededChild(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "budget-continue", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	now := time.Now()
	_ = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "done", "labels": issueLabelsColumn([]string{"budget_exceeded"}), "execution_phase": "budget_exceeded", "result": "promising evidence", "completed_at": now}).Error
	if err = bridge.ContinueTerminalChildIssue(context.Background(), coordination.Invocation{IssueID: root.ID, AgentID: root.AssigneeAgentID, ExecutionID: "parent-execution"}, coordination.ContinueRequest{ChildIssueID: child.ID, Reason: "证据有价值，继续验证"}); err != nil {
		t.Fatal(err)
	}
	reopened, _ := store.GetIssue(child.ID)
	if reopened.Status != "todo" || reopened.ExecutionPhase != "scheduled" || reopened.CompletedAt != nil {
		t.Fatalf("child was not reopened: %+v", reopened)
	}
	var pending int64
	if err = store.db.Table("coordination_events").Where("payload LIKE ? AND payload LIKE ?", "%"+child.ID+"%", "%"+string(coordination.EventIssueAssigned)+"%").Count(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("assignment events=%d, want 1", pending)
	}
	failedChild, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Failed child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	_ = store.db.Model(&Issue{}).Where("id = ?", failedChild.ID).Updates(map[string]any{"status": "done", "labels": issueLabelsColumn([]string{"failed"}), "execution_phase": "completed", "error": "provider unavailable", "completed_at": now}).Error
	if err = bridge.ContinueTerminalChildIssue(context.Background(), coordination.Invocation{IssueID: root.ID, AgentID: root.AssigneeAgentID, ExecutionID: "parent-execution"}, coordination.ContinueRequest{ChildIssueID: failedChild.ID, Reason: "临时错误，可以恢复"}); err != nil {
		t.Fatal(err)
	}
	retried, _ := store.GetIssue(failedChild.ID)
	if retried.Status != "todo" || retried.Error != "" || retried.CompletedAt != nil {
		t.Fatalf("failed child was not reopened cleanly: %+v", retried)
	}
}

func TestTaskTimeBudgetRejectsNonPositiveValue(t *testing.T) {
	store := configuredStore(t)
	minutes := 0
	if _, _, err := store.CreateTask(CreateIssueInput{Title: "Invalid budget", Priority: "middle", WorkMode: "autonomous", TimeBudgetMinutes: &minutes}); err == nil {
		t.Fatal("non-positive task budget was accepted")
	}
}

func TestWorkerPromptRequiresBudgetSizedDecomposition(t *testing.T) {
	prompt := workerPrompt(Issue{Identifier: "AEG-1", Title: "large audit", Workspace: TaskWorkspacePath}, 3, 4, 8, IssueBudgetConfig{MaxTurns: 37, ActiveTimeMinutes: 11})
	for _, expected := range []string{"37 model turns", "11 active minutes", "tens-of-thousands-of-lines", "phone_board_delegate"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("worker prompt missing %q: %s", expected, prompt)
		}
	}
}

func agenthostSpecForTest() agenthost.ExecutionSpec {
	return agenthost.ExecutionSpec{ExecutionID: "test", AgentID: "test", Prompt: "test", Model: agenthost.ModelRef{Provider: "test", Model: "test"}}
}
