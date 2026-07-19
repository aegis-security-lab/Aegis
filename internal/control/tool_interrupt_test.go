package control

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type toolInterruptCaptureWriter struct {
	bytes.Buffer
}

func (w *toolInterruptCaptureWriter) Close() error { return nil }

func TestInterruptCurrentToolKeepsIssueAndSessionActive(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Run a long test", Objective: "The test completes successfully.",
		Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CheckoutIssue(issue.ID, CheckoutIssueInput{
		AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running", "current_tool": "bash", "pid": 123}); err != nil {
		t.Fatal(err)
	}

	writer := &toolInterruptCaptureWriter{}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{
		manager: manager, key: execution.ID, executionID: execution.ID,
		issueID: issue.ID, agentID: "backend-engineer", kind: "work", stdin: writer,
	}
	session.busy.Store(true)
	session.beginTool("bash")
	manager.sessions[execution.ID] = session

	result, err := manager.InterruptCurrentTool(execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tool != "bash" || result.Status != "interrupting" {
		t.Fatalf("unexpected interrupt result: %+v", result)
	}
	var interrupting Execution
	if err = store.db.First(&interrupting, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if interrupting.CurrentTool != "" || !strings.Contains(interrupting.Checkpoint, "正在中断") {
		t.Fatalf("interrupt request was not reflected immediately: %+v", interrupting)
	}
	line := bytes.Split(bytes.TrimSpace(writer.Bytes()), []byte("\n"))[0]
	var command map[string]any
	if err = json.Unmarshal(line, &command); err != nil {
		t.Fatal(err)
	}
	if command["type"] != "abort" {
		t.Fatalf("Pi command=%+v, want abort", command)
	}

	manager.handleRPCLine(session, []byte(`{"type":"agent_settled"}`))
	updatedIssue, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var updatedExecution Execution
	if err = store.db.First(&updatedExecution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedIssue.Status != "in_progress" || updatedIssue.CheckoutExecutionID != execution.ID {
		t.Fatalf("interrupt released or completed Issue: %+v", updatedIssue)
	}
	if updatedExecution.Status != "running" || updatedExecution.CurrentTool != "" || updatedExecution.FinishedAt != nil {
		t.Fatalf("interrupt finalized Execution: %+v", updatedExecution)
	}
	if !strings.Contains(updatedExecution.Checkpoint, "等待新的继续指令") {
		t.Fatalf("unexpected interrupt checkpoint: %q", updatedExecution.Checkpoint)
	}
}

func TestInterruptCurrentToolRejectsValidationSession(t *testing.T) {
	store := configuredStore(t)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{executionID: "validation-execution", kind: "validation", stdin: &toolInterruptCaptureWriter{}}
	session.busy.Store(true)
	manager.sessions[session.executionID] = session
	if _, err := manager.InterruptCurrentTool(session.executionID); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("expected validation interrupt rejection, got %v", err)
	}
}
