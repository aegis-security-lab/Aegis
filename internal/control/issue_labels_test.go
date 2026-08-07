package control

import (
	"testing"
	"time"
)

func TestLegacyIssueStatusMigrationRewritesStatusesToLabels(t *testing.T) {
	store := configuredStore(t)
	now := time.Now()
	legacy := map[string]struct {
		expectedStatus string
		expectedLabels []string
	}{
		"backlog":         {"todo", nil},
		"blocked":         {"in_progress", []string{"blocked"}},
		"failed":          {"done", []string{"failed"}},
		"budget_exceeded": {"done", []string{"budget_exceeded"}},
	}
	issueIDs := make(map[string]string, len(legacy))
	for status := range legacy {
		issue, err := store.CreateIssue(CreateIssueInput{Title: "Legacy " + status, Priority: "medium", WorkMode: "autonomous", Status: status, AssigneeAgentID: "backend-engineer"})
		if err != nil {
			t.Fatalf("create legacy %s: %v", status, err)
		}
		issueIDs[status] = issue.ID
		// Rewrite directly in the new model vocabulary to simulate pre-migration rows.
		_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
			"status": status, "labels": issueLabelsColumn(nil),
		}).Error
	}
	if err := migrateLegacyIssueStatuses(store.db, now); err != nil {
		t.Fatal(err)
	}
	for status, want := range legacy {
		issue, err := store.GetIssue(issueIDs[status])
		if err != nil {
			t.Fatal(err)
		}
		if issue.Status != want.expectedStatus {
			t.Fatalf("%s: status = %q, want %q", status, issue.Status, want.expectedStatus)
		}
		if len(issue.Labels) != len(want.expectedLabels) {
			t.Fatalf("%s: labels = %v, want %v", status, issue.Labels, want.expectedLabels)
		}
		for i, label := range want.expectedLabels {
			if issue.Labels[i] != label {
				t.Fatalf("%s: labels = %v, want %v", status, issue.Labels, want.expectedLabels)
			}
		}
		if status == "backlog" && issue.ExecutionPhase != "active" {
			t.Fatalf("backlog migration left phase %q", issue.ExecutionPhase)
		}
		if want.expectedStatus == "done" && issue.CompletedAt == nil {
			t.Fatalf("%s: done issue missing completed_at", status)
		}
	}
	// A second run must be idempotent.
	if err := migrateLegacyIssueStatuses(store.db, now); err != nil {
		t.Fatal(err)
	}
	for status, want := range legacy {
		issue, err := store.GetIssue(issueIDs[status])
		if err != nil {
			t.Fatal(err)
		}
		if issue.Status != want.expectedStatus || len(issue.Labels) != len(want.expectedLabels) {
			t.Fatalf("%s: second migration changed %+v", status, issue)
		}
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
