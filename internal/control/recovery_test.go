package control

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
	if err = reopened.db.First(&approval, "id = ?", approval.ID).Error; err != nil || approval.Status != "expired" {
		t.Fatalf("stale approval was not expired: err=%v approval=%+v", err, approval)
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

func TestRecoveringIssueCannotBeDispatchedAsNewWork(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{
		Title: "Recover only", Objective: "Resume existing work.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "todo", "execution_phase": "recovering", "recovery_execution_id": "execution-existing",
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err := manager.dispatchIssue(issue.ID); err == nil || !strings.Contains(err.Error(), "恢复") {
		t.Fatalf("recovering Issue was dispatched as new work: %v", err)
	}
}
