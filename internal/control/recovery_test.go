package control

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPlanningSessionAcceptsDurableToolPlanWithMarkdownFinalResponse(t *testing.T) {
	store := configuredStore(t)
	parent, execution, decomposition := createPlanningToolResult(t, store)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	defer func() {
		_ = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Update("status", "cancelled").Error
		manager.scheduleMu.Unlock()
	}()

	manager.handlePlan(parent, &PiSession{
		executionID: execution.ID, issueID: parent.ID, agentID: execution.AgentID, kind: "planning",
	}, "# Planning complete\n\nCreated two independently verifiable child Issues.")

	if err := store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" || execution.Error != "" || !strings.HasPrefix(execution.Result, "# Planning complete") {
		t.Fatalf("planning execution was not completed from its durable tool result: %+v", execution)
	}
	updated, err := store.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_progress" || updated.ExecutionPhase != "waiting_children" || updated.Error != "" {
		t.Fatalf("parent did not remain in waiting_children: %+v", updated)
	}
	var issueCount int64
	store.db.Model(&Issue{}).Where("hidden = ?", false).Count(&issueCount)
	if issueCount != 3 || len(decomposition.ChildIDs) != 2 {
		t.Fatalf("tool plan was duplicated: issues=%d decomposition=%+v", issueCount, decomposition)
	}
}

func TestStartupRepairsFailedPlanningParseAfterToolCreatedChildren(t *testing.T) {
	store := configuredStore(t)
	parent, execution, _ := createPlanningToolResult(t, store)
	now := time.Now()
	parseError := "执行计划无法解析: invalid character '#' looking for beginning of value"
	if err := store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": parseError, "finished_at": now}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
		"status": "in_progress", "labels": issueLabelsColumn([]string{"blocked"}), "execution_phase": "blocked", "error": parseError,
		"current_execution_id": execution.ID, "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	message := Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: parent.ID, Role: "assistant",
		Content: "# Plan created\n\nThe durable child tree is ready.", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	defer func() {
		_ = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Update("status", "cancelled").Error
		manager.scheduleMu.Unlock()
	}()
	manager.reconcileCompletedPlanningTools()

	if err := store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" || execution.Error != "" || execution.Result != message.Content {
		t.Fatalf("failed planning execution was not repaired: %+v", execution)
	}
	updated, err := store.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_progress" || updated.ExecutionPhase != "waiting_children" || updated.Error != "" {
		t.Fatalf("blocked parent was not repaired: %+v", updated)
	}
	var recoveryEvents int64
	store.db.Model(&ExecutionEvent{}).Where("execution_id = ? AND type = ?", execution.ID, "recovery").Count(&recoveryEvents)
	if recoveryEvents != 1 {
		t.Fatalf("recovery events=%d, want 1", recoveryEvents)
	}
}

func TestStartupMigratesBlockedExecutionFailureToTerminalIssue(t *testing.T) {
	store := configuredStore(t)
	dataDir := store.DataDir()
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Legacy blocked failure", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": "runtime failed", "finished_at": now}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "blocked", "execution_phase": "blocked", "current_execution_id": execution.ID,
		"checkout_execution_id": execution.ID, "error": "runtime failed", "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Status != "done" || !hasIssueLabel(migrated, issueLabelFailed) || migrated.ExecutionPhase != "completed" || migrated.CompletedAt == nil || migrated.CheckoutExecutionID != "" {
		t.Fatalf("legacy blocked execution failure was not migrated: %+v", migrated)
	}
}

func createPlanningToolResult(t *testing.T, store *Store) (Issue, Execution, IssueDecomposition) {
	t.Helper()
	parent, err := store.CreateIssue(CreateIssueInput{
		Title: "Plan a broad implementation", Objective: "Deliver an integrated implementation.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "aegis-orchestrator",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(parent, "aegis-orchestrator", "planning")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CheckoutIssue(parent.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	result, err := store.CreateSubIssues(parent.ID, execution.ID, execution.AgentID, DecomposeIssueInput{
		RequestKey: "initial-plan", Summary: "Split backend and frontend work.",
		Children: []SubIssueSpec{
			{Title: "Implement backend", Objective: "Backend tests pass.", Priority: "high", AgentID: "backend-engineer"},
			{Title: "Implement frontend", Objective: "Frontend build passes.", Priority: "medium", AgentID: "frontend-engineer", DependsOn: []int{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return parent, execution, result.Decomposition
}

func TestRuntimeRecordsToolCheckpointsForRecovery(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{
		Title: "Checkpoint tools", Objective: "Record progress.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	execution, _ := store.createExecution(issue, "backend-engineer", "work")
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{}}
	started, _ := json.Marshal(map[string]any{
		"type": "tool_execution_start", "toolCallId": "call-checkpoint", "toolName": "bash",
		"args": map[string]any{"command": "go test ./..."},
	})
	manager.handleRPCLine(session, started)
	if err := store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "running" || execution.CurrentTool != "bash" || !strings.Contains(execution.Checkpoint, "go test ./...") || execution.CheckpointAt == nil {
		t.Fatalf("tool start checkpoint was not recorded: %+v", execution)
	}
	finished, _ := json.Marshal(map[string]any{
		"type": "tool_execution_end", "toolCallId": "call-checkpoint", "toolName": "bash", "result": "ok", "isError": false,
	})
	manager.handleRPCLine(session, finished)
	if err := store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.CurrentTool != "" || execution.Checkpoint != "已完成工具调用 bash" {
		t.Fatalf("tool completion checkpoint was not recorded: %+v", execution)
	}
}

func TestRestartPersistsInterruptedExecutionForAutomaticRecovery(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Interrupted backend work", Objective: "Finish the backend implementation.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	originalSessionID := execution.SessionID
	checkpoint := "正在调用 bash：go test ./..."
	if err = store.updateExecution(execution.ID, map[string]any{
		"status": "running", "initial_prompt": "Implement and test the backend.",
		"checkpoint": checkpoint, "checkpoint_at": time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	message := Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: "I updated the handler and am running integration tests.", Streaming: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err = store.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	approval := Approval{
		ID: nextID("approval"), ExecutionID: execution.ID, IssueID: issue.ID, Type: "tool_call",
		Title: "Run integration tests", Status: "pending", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}
	store.startToolEvent(execution.ID, issue.ID, "call-running", "bash", map[string]any{"command": "go test ./..."})

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	recovering, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovering.Status != "todo" || recovering.ExecutionPhase != "recovering" || recovering.RecoveryExecutionID != execution.ID || recovering.RecoveryPhase != "active" || recovering.RecoveryRequestedAt == nil {
		t.Fatalf("interrupted Issue was not queued for recovery: %+v", recovering)
	}
	if err = reopened.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "disconnected" || execution.SessionID != originalSessionID || execution.Checkpoint != checkpoint {
		t.Fatalf("execution identity/checkpoint was not preserved: %+v", execution)
	}
	if err = reopened.db.First(&message, "id = ?", message.ID).Error; err != nil || message.Streaming {
		t.Fatalf("streaming message was not safely closed: err=%v message=%+v", err, message)
	}
	var removedApprovalCount int64
	if err = reopened.db.Model(&Approval{}).Where("id = ?", approval.ID).Count(&removedApprovalCount).Error; err != nil || removedApprovalCount != 0 {
		t.Fatalf("invalid approvals were not removed: err=%v count=%d", err, removedApprovalCount)
	}
	var toolEvent ExecutionEvent
	if err = reopened.db.Where("execution_id = ? AND tool_call_id = ?", execution.ID, "call-running").First(&toolEvent).Error; err != nil || toolEvent.Status != "interrupted" {
		t.Fatalf("running tool event was not marked interrupted: err=%v event=%+v", err, toolEvent)
	}

	manager := &Manager{store: reopened, controlURL: "http://127.0.0.1:8080", sessions: map[string]*PiSession{}}
	checkedOut, recoveredExecution, agent, prompt, err := manager.prepareIssueRecovery(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkedOut.Status != "in_progress" || checkedOut.ExecutionPhase != "active" || checkedOut.CheckoutExecutionID != execution.ID {
		t.Fatalf("Issue was not checked out to the original execution: %+v", checkedOut)
	}
	if recoveredExecution.ID != execution.ID || recoveredExecution.SessionID != originalSessionID || agent.ID != "backend-engineer" {
		t.Fatalf("recovery created a different execution/session: execution=%+v agent=%+v", recoveredExecution, agent)
	}
	for _, expected := range []string{checkpoint, message.Content, "Implement and test the backend", "Do not repeat completed work"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("recovery prompt is missing %q: %s", expected, prompt)
		}
	}
}

func TestRestartFinalizesSettledWorkerWithoutRepeatingWork(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Worker settled before Issue finalization", Objective: "Preserve the completed worker result.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "rework")
	if err != nil {
		t.Fatal(err)
	}
	result := "Implementation and verification are complete."
	now := time.Now()
	if err = store.updateExecution(execution.ID, map[string]any{
		"status": "completed", "result": result, "finished_at": now,
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "validation_disabled": true,
		"checkout_execution_id": execution.ID, "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	recovering, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovering.Status != "todo" || recovering.ExecutionPhase != "recovering" || recovering.RecoveryPhase != "settled" || recovering.RecoveryExecutionID != execution.ID {
		t.Fatalf("settled worker was not queued for finalization recovery: %+v", recovering)
	}
	if err = reopened.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" || execution.Result != result {
		t.Fatalf("settled worker was incorrectly converted into interrupted work: %+v", execution)
	}

	manager := &Manager{store: reopened, sessions: map[string]*PiSession{}}
	if err = manager.recoverIssueLocked(recovering); err != nil {
		t.Fatal(err)
	}
	finished, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "done" || finished.ExecutionPhase != "completed" || finished.Result != result || finished.CheckoutExecutionID != "" || finished.RecoveryExecutionID != "" {
		t.Fatalf("settled worker result was not finalized after restart: %+v", finished)
	}
}

func TestRestartPreservesDurableSleepingIssue(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Parent waiting durably", Objective: "Resume only after the durable wake signal.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "wakeup")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running", "runtime_type": "agentcore"}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "sleeping", "sleep_token": "durable-wait",
		"checkout_execution_id": "", "current_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := reopened.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Status != "in_progress" || preserved.ExecutionPhase != "sleeping" || preserved.SleepToken != "durable-wait" || preserved.RecoveryExecutionID != "" {
		t.Fatalf("durable sleep was incorrectly converted to restart recovery: %+v", preserved)
	}
}

func TestRestartResumesSameValidationAttemptAndSession(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Interrupted validation", Objective: "The implementation and tests are complete.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	validator, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	originalSessionID := validator.SessionID
	if err = store.updateExecution(validator.ID, map[string]any{"status": "running", "checkpoint": "正在核对集成测试证据", "checkpoint_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID, ValidationExecutionID: validator.ID,
		Attempt: 2, Objective: issue.Objective, CandidateResult: "Implementation complete; tests pass.", Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "validating", "current_execution_id": validator.ID,
		"validation_execution_id": validator.ID, "result": validation.CandidateResult,
	}).Error; err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	recovering, _ := reopened.GetIssue(issue.ID)
	if recovering.ExecutionPhase != "recovering" || recovering.RecoveryExecutionID != validator.ID || recovering.RecoveryPhase != "validating" {
		t.Fatalf("validation Issue was not queued for recovery: %+v", recovering)
	}
	if err = reopened.db.First(&validation, "id = ?", validation.ID).Error; err != nil || validation.Status != "interrupted" || validation.Attempt != 2 {
		t.Fatalf("validation attempt was not interrupted durably: err=%v validation=%+v", err, validation)
	}

	manager := &Manager{store: reopened, controlURL: "http://127.0.0.1:8080", sessions: map[string]*PiSession{}}
	checkedOut, recoveredExecution, agent, prompt, err := manager.prepareIssueRecovery(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkedOut.ExecutionPhase != "validating" || checkedOut.Status != "in_progress" || recoveredExecution.ID != validator.ID || recoveredExecution.SessionID != originalSessionID || agent.ID != "acceptance-validator" {
		t.Fatalf("validation session was not restored in place: issue=%+v execution=%+v agent=%+v", checkedOut, recoveredExecution, agent)
	}
	if err = reopened.db.First(&validation, "id = ?", validation.ID).Error; err != nil || validation.Status != "running" || validation.Attempt != 2 {
		t.Fatalf("validation recovery changed the attempt: err=%v validation=%+v", err, validation)
	}
	if !strings.Contains(prompt, "不要把重启算作新的尝试") || !strings.Contains(prompt, validation.CandidateResult) {
		t.Fatalf("unexpected validation recovery prompt: %s", prompt)
	}
}

func TestOrphanedValidationInfrastructureFailureIsRecoveredInPlace(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Orphaned validation", Objective: "Verify the published evidence.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	validator, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	originalSessionID := validator.SessionID
	if err = store.updateExecution(validator.ID, map[string]any{
		"status": "failed", "error": "验收附件 report.md 在服务端不存在或不完整", "finished_at": time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID,
		ValidationExecutionID: validator.ID, Attempt: 1, Objective: issue.Objective,
		CandidateResult: "Published evidence", Status: "interrupted", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "done", "labels": issueLabelsColumn([]string{"failed"}), "execution_phase": "completed", "current_execution_id": validator.ID,
		"validation_execution_id": validator.ID, "error": "验收附件 report.md 在服务端不存在或不完整", "completed_at": time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.reconcileOrphanedValidations()
	recovering, err := store.GetIssue(issue.ID)
	if err != nil || recovering.Status != "todo" || recovering.ExecutionPhase != "recovering" || recovering.RecoveryExecutionID != validator.ID || recovering.CompletedAt != nil {
		t.Fatalf("orphaned Issue was not made recoverable: err=%v issue=%+v", err, recovering)
	}
	if err = store.db.First(&validation, "id = ?", validation.ID).Error; err != nil || validation.Status != "interrupted" || validation.Attempt != 1 {
		t.Fatalf("validation attempt was not preserved: err=%v validation=%+v", err, validation)
	}
	if err = store.db.First(&validator, "id = ?", validator.ID).Error; err != nil || validator.Status != "disconnected" || validator.SessionID != originalSessionID {
		t.Fatalf("validation Execution was not preserved: err=%v execution=%+v", err, validator)
	}

	checkedOut, recoveredExecution, agent, prompt, err := manager.prepareIssueRecovery(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkedOut.Status != "in_progress" || checkedOut.ExecutionPhase != "validating" || recoveredExecution.ID != validator.ID || recoveredExecution.SessionID != originalSessionID || agent.ID != "acceptance-validator" {
		t.Fatalf("validation did not resume in place: issue=%+v execution=%+v agent=%+v", checkedOut, recoveredExecution, agent)
	}
	if !strings.Contains(prompt, "不要把重启算作新的尝试") {
		t.Fatalf("unexpected recovery prompt: %s", prompt)
	}
}

func TestManagerDefersRecoveryUntilCoordinationIsAvailable(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Deferred validation recovery", Objective: "Verify evidence after restart.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := store.createExecution(issue, "backend-engineer", "work")
	validator, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(validator.ID, map[string]any{"status": "disconnected", "runtime_type": "container", "finished_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID,
		ValidationExecutionID: validator.ID, Attempt: 1, Objective: issue.Objective,
		CandidateResult: "evidence", Status: "interrupted", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "todo", "execution_phase": "recovering", "current_execution_id": validator.ID,
		"validation_execution_id": validator.ID, "recovery_execution_id": validator.ID,
		"recovery_phase": "validating", "recovery_requested_at": time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	before, _ := store.GetIssue(issue.ID)
	if before.Status != "todo" || before.ExecutionPhase != "recovering" {
		t.Fatalf("Manager recovered before Coordination was installed: %+v", before)
	}
	bridge, err := newTestCoordinationBridge(manager, "deferred-recovery", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	deadline := time.Now().Add(3 * time.Second)
	for {
		after, loadErr := store.GetIssue(issue.ID)
		if loadErr == nil && after.Status == "in_progress" && after.ExecutionPhase == "validating" && after.RecoveryExecutionID == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Manager did not recover after Coordination became available: err=%v issue=%+v", loadErr, after)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentCoreRecoveryFallsBackAfterPreparedEnvelopeFails(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Recover after prepared envelope failure", Objective: "Resume the same AgentCore execution.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "rework")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "disconnected", "runtime_type": "agentcore", "finished_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "todo", "execution_phase": "recovering", "current_execution_id": execution.ID,
		"recovery_execution_id": execution.ID, "recovery_phase": "active", "recovery_requested_at": time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	bridge, err := newTestCoordinationBridge(manager, "agentcore-recovery-fallback", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })

	now := time.Now().UTC()
	payload := fmt.Sprintf(`{"id":"old-prepared","coordinationId":"%s","spec":{"executionId":"old-prepared","agentId":"%s","values":{"control.preparedExecutionId":"%s"}},"status":"failed","attempt":1,"maxAttempts":1,"priority":100,"availableAt":%q,"createdAt":%q,"updatedAt":%q}`, issue.ID, issue.AssigneeAgentID, execution.ID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err = store.db.Exec(`INSERT INTO coordination_executions (id, coordination_id, status, priority, available_at, created_at, updated_at, payload) VALUES (?, ?, 'failed', 100, ?, ?, ?, ?)`, "old-prepared", issue.ID, now, now, now, payload).Error; err != nil {
		t.Fatal(err)
	}

	manager.resumeInterruptedWork()
	recovered, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "in_progress" || recovered.ExecutionPhase != "active" || recovered.RecoveryExecutionID != "" || recovered.CurrentExecutionID != execution.ID {
		t.Fatalf("failed prepared envelope did not fall back to recovery: %+v", recovered)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil || execution.Status != "queued" {
		t.Fatalf("original execution was not requeued: err=%v execution=%+v", err, execution)
	}
}
