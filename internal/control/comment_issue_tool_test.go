package control

import "testing"

func TestCommentIssueFromExecutionIsTaskScopedAndWakesDifferentAssignee(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateIssue(CreateIssueInput{Title: "Other task", Priority: "low", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{key: "execution-parent", executionID: "execution-parent", issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}
	manager.sessions[session.key] = session
	manager.scheduleMu.Lock()
	defer manager.scheduleMu.Unlock()

	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active"}).Error; err != nil {
		t.Fatal(err)
	}
	comment, err := manager.CommentIssueFromExecution(session.executionID, "secret", CommentIssueInput{IssueID: child.ID, Body: "Please verify the newly discovered response path."})
	if err != nil {
		t.Fatal(err)
	}
	if comment.AuthorType != "agent" || comment.AuthorID != "backend-engineer" || comment.IssueID != child.ID {
		t.Fatalf("unexpected comment: %+v", comment)
	}
	if len(comment.WakeupIDs) != 1 {
		t.Fatalf("comment did not return its wakeup id: %+v", comment)
	}
	var wakeup AgentWakeup
	if err = store.db.Where("comment_id = ?", comment.ID).First(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.AgentID != "frontend-engineer" || wakeup.Reason != "agent_comment_assignee" || wakeup.Status != "queued" {
		t.Fatalf("unexpected wakeup: %+v", wakeup)
	}
	if comment.WakeupIDs[0] != wakeup.ID {
		t.Fatalf("returned wakeup id=%q, want %q", comment.WakeupIDs[0], wakeup.ID)
	}
	if _, err = manager.CommentIssueFromExecution(session.executionID, "secret", CommentIssueInput{IssueID: child.ID, Body: "A second pending comment."}); err == nil {
		t.Fatal("child accepted another comment while the first wakeup was pending")
	}
	_ = store.db.Model(&AgentWakeup{}).Where("id = ?", wakeup.ID).Update("status", "cancelled").Error

	if _, err = manager.CommentIssueFromExecution(session.executionID, "secret", CommentIssueInput{IssueID: other.ID, Body: "Cross-task comment"}); err == nil {
		t.Fatal("cross-task comment was accepted")
	}
	if _, err = manager.CommentIssueFromExecution(session.executionID, "secret", CommentIssueInput{IssueID: parent.ID, Body: "Record current evidence."}); err == nil {
		t.Fatal("commenting the current Issue instead of a direct child was accepted")
	}
}
