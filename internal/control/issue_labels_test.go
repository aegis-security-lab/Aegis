package control

import (
	"testing"
	"time"
)

func TestIssueWritesRejectRetiredWorkflowAliases(t *testing.T) {
	store := configuredStore(t)
	for _, input := range []CreateIssueInput{
		{Title: "Retired status", Priority: "middle", Status: "backlog", WorkMode: "autonomous"},
		{Title: "Retired priority", Priority: "medium", Status: "todo", WorkMode: "autonomous"},
	} {
		if _, err := store.CreateIssue(input); err == nil {
			t.Fatalf("retired workflow value was accepted: %+v", input)
		}
	}

	issue, err := store.CreateIssue(CreateIssueInput{Title: "Canonical workflow", Priority: "middle", Status: "todo", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	retiredStatus := "failed"
	if _, err = store.UpdateIssue(issue.ID, UpdateIssueInput{Status: &retiredStatus}); err == nil {
		t.Fatal("retired update status was accepted")
	}
	retiredPriority := "critical"
	if _, err = store.UpdateIssue(issue.ID, UpdateIssueInput{Priority: &retiredPriority}); err == nil {
		t.Fatal("retired update priority was accepted")
	}
}

func TestUpdateIssueReopenDropsOperationalLabels(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Fail then reopen", Priority: "high", WorkMode: "autonomous", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "done", "labels": issueLabelsColumn([]string{"failed"}), "execution_phase": "completed",
		"completed_at": time.Now(),
	}).Error
	done, err := store.GetIssue(issue.ID)
	if err != nil || done.Status != "done" || !hasIssueLabel(done, issueLabelFailed) {
		t.Fatalf("setup failed: %+v", done)
	}
	todo := "todo"
	reopened, err := store.UpdateIssue(issue.ID, UpdateIssueInput{Status: &todo})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "todo" || hasIssueLabel(reopened, issueLabelFailed) || reopened.Error != "" || reopened.CompletedAt != nil {
		t.Fatalf("reopen did not clear failed outcome: %+v", reopened)
	}
}
