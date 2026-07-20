package control

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestIssueCompletionGuardContinuesSameSessionForUnfinishedChildren(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Still running", Objective: "Finish evidence", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, _ := store.createExecution(parent, "backend-engineer", "continuation")
	parent, _ = store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"}})
	var input bytes.Buffer
	session := &PiSession{key: execution.ID, executionID: execution.ID, issueID: parent.ID, agentID: "backend-engineer", kind: "continuation", stdin: nopWriteCloser{Writer: &input}}
	manager := &Manager{store: store, sessions: map[string]*PiSession{execution.ID: session}}

	if !manager.continueForUnfinishedChildren(parent, session) {
		t.Fatal("completion guard did not continue the session")
	}
	updated, _ := store.GetIssue(parent.ID)
	if updated.Status != "in_progress" || updated.ExecutionPhase != "active" || updated.CheckoutExecutionID != execution.ID {
		t.Fatalf("parent did not remain active: %+v", updated)
	}
	var message Message
	if err := store.db.Where("execution_id = ? AND role = ?", execution.ID, "user").Order("created_at desc").First(&message).Error; err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{child.ID, child.Identifier, "aegis_wait_for_child_issues", "aegis_cancel_issue"} {
		if !strings.Contains(message.Content, expected) {
			t.Fatalf("guard prompt missing %q: %s", expected, message.Content)
		}
	}
	if !strings.Contains(input.String(), "\"type\":\"prompt\"") {
		t.Fatalf("guard prompt was not sent to the same Pi session: %s", input.String())
	}
}

func TestAgentCanSummarizeBeforeCancellingDirectChild(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Partial child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	parentExecution, _ := store.createExecution(parent, "backend-engineer", "continuation")
	childExecution, _ := store.createExecution(child, "frontend-engineer", "work")
	_, _ = store.CheckoutIssue(child.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: childExecution.ID, ExpectedStatuses: []string{"todo"}})
	_ = store.updateExecution(childExecution.ID, map[string]any{"status": "running"})
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[parentExecution.ID] = &PiSession{manager: manager, executionID: parentExecution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}
	childSession := &PiSession{manager: manager, executionID: childExecution.ID, issueID: child.ID, agentID: "frontend-engineer", stdin: &trackedWriteCloser{}}
	manager.sessions[childExecution.ID] = childSession

	requested, err := manager.CancelIssueFromExecution(parentExecution.ID, "secret", CancelIssueInput{IssueID: child.ID, Reason: "Scope replaced", Mode: "summarize_then_cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if requested.Status != "in_progress" || requested.ExecutionPhase != "summarizing" || requested.AbandonRequestedAt == nil {
		t.Fatalf("child did not enter summarizing: %+v", requested)
	}
	summary := "已完成信息收集并保存证据；剩余风险是尚未复核。"
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: childExecution.ID, IssueID: child.ID, Role: "assistant", Content: summary, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	manager.handleSettled(childSession)
	finished, _ := store.GetIssue(child.ID)
	if finished.Status != "cancelled" || finished.Result != summary || !finished.ObjectiveAbandoned {
		t.Fatalf("summary was not preserved before cancellation: %+v", finished)
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestAgentCanCancelOnlyDirectChildAndItsSubtree(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Obsolete child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	grandchild, _ := store.CreateIssue(CreateIssueInput{ParentID: child.ID, Title: "Nested work", Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	other, _ := store.CreateIssue(CreateIssueInput{Title: "Other task", Priority: "low", WorkMode: "autonomous"})
	parentExecution, _ := store.createExecution(parent, "backend-engineer", "continuation")
	parent, _ = store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: "backend-engineer", ExecutionID: parentExecution.ID, ExpectedStatuses: []string{"todo"}})
	childExecution, _ := store.createExecution(child, "frontend-engineer", "work")
	_, _ = store.CheckoutIssue(child.ID, CheckoutIssueInput{AgentID: "frontend-engineer", ExecutionID: childExecution.ID, ExpectedStatuses: []string{"todo"}})
	_ = store.updateExecution(childExecution.ID, map[string]any{"status": "running"})
	manager := &Manager{store: store, sessions: map[string]*PiSession{parentExecution.ID: {key: parentExecution.ID, executionID: parentExecution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}}}

	cancelled, err := manager.CancelIssueFromExecution(parentExecution.ID, "secret", CancelIssueInput{IssueID: child.ID, Reason: "Superseded by verified coverage in another child.", Mode: "immediate"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("child was not cancelled: %+v", cancelled)
	}
	grandchild, _ = store.GetIssue(grandchild.ID)
	if grandchild.Status != "cancelled" {
		t.Fatalf("unfinished descendant was not cancelled: %+v", grandchild)
	}
	if err = store.db.First(&childExecution, "id = ?", childExecution.ID).Error; err != nil || childExecution.Status != "cancelled" {
		t.Fatalf("active child execution was not cancelled: err=%v execution=%+v", err, childExecution)
	}
	if _, err = manager.CancelIssueFromExecution(parentExecution.ID, "secret", CancelIssueInput{IssueID: other.ID, Reason: "Not allowed", Mode: "immediate"}); err == nil {
		t.Fatal("Agent cancelled an Issue outside its direct children")
	}
}
