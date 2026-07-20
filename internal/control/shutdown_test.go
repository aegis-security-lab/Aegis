package control

import "testing"

func TestSessionExitDuringShutdownRemainsRecoverable(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Recover worker after service restart", Objective: "Resume the interrupted worker.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CheckoutIssue(issue.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, kind: "work"}
	manager.sessions[session.key] = session
	manager.BeginShutdown()
	manager.sessionExited(session, nil)

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_progress" || updated.ExecutionPhase != "active" || updated.CheckoutExecutionID != execution.ID {
		t.Fatalf("shutdown exit made Issue unrecoverable: %+v", updated)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "running" {
		t.Fatalf("shutdown exit changed execution status: %+v", execution)
	}
}
