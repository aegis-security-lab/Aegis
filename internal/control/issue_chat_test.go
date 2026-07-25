package control

import (
	"testing"
	"time"
)

func TestChatExecutionDoesNotPublishIssueCommentOrChangeIssueState(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Discuss completed work", Objective: "Keep the accepted result unchanged.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "done", "execution_phase": "completed", "result": "accepted result", "completed_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&Message{
		ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant",
		Content: "This answer belongs only to the chat timeline.", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.handleSettled(&PiSession{executionID: execution.ID, issueID: issue.ID, agentID: execution.AgentID, kind: "chat"})

	var commentCount int64
	if err = store.db.Model(&IssueComment{}).Where("issue_id = ?", issue.ID).Count(&commentCount).Error; err != nil {
		t.Fatal(err)
	}
	if commentCount != 0 {
		t.Fatalf("chat completion created %d Issue comments, want 0", commentCount)
	}
	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "done" || updated.ExecutionPhase != "completed" || updated.Result != "accepted result" {
		t.Fatalf("chat completion changed Issue state: %+v", updated)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" || execution.Result != "This answer belongs only to the chat timeline." {
		t.Fatalf("chat execution was not recorded correctly: %+v", execution)
	}
}
