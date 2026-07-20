package control

import (
	"testing"
	"time"
)

func TestWakeupExecutionMakesIssueActiveThenRestoresPriorTerminalState(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Completed discussion", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "checkout_execution_id": ""}).Error; err != nil {
		t.Fatal(err)
	}
	issue, _ = store.GetIssue(issue.ID)
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, CommentID: nextID("comment"), AgentID: "backend-engineer", Status: "queued", CreatedAt: time.Now()}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err = manager.markIssueForWakeup(&wakeup, issue, "execution-wakeup", time.Now()); err != nil {
		t.Fatal(err)
	}
	active, _ := store.GetIssue(issue.ID)
	if active.Status != "in_progress" || active.ExecutionPhase != "active" || active.CheckoutExecutionID != "execution-wakeup" || active.CurrentExecutionID != "execution-wakeup" {
		t.Fatalf("wakeup did not make Issue active: %+v", active)
	}
	if wakeup.PriorIssueStatus != "done" || wakeup.PriorExecutionPhase != "completed" || wakeup.Status != "delivered" {
		t.Fatalf("wakeup did not preserve prior state: %+v", wakeup)
	}

	manager.restoreIssueAfterWakeup(issue.ID, "execution-wakeup")
	restored, _ := store.GetIssue(issue.ID)
	if restored.Status != "done" || restored.ExecutionPhase != "completed" || restored.CheckoutExecutionID != "" {
		t.Fatalf("pure wakeup did not restore prior terminal state: %+v", restored)
	}
}

func TestWakeupDoesNotRestoreOverNewWaitingChildrenState(t *testing.T) {
	store := configuredStore(t)
	issue, _ := store.CreateIssue(CreateIssueInput{Title: "Reopened parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed"}).Error
	issue, _ = store.GetIssue(issue.ID)
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, AgentID: "backend-engineer", Status: "queued", CreatedAt: time.Now()}
	_ = store.db.Create(&wakeup).Error
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err := manager.markIssueForWakeup(&wakeup, issue, "execution-wakeup", time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("execution_phase", "waiting_children").Error
	manager.restoreIssueAfterWakeup(issue.ID, "execution-wakeup")
	updated, _ := store.GetIssue(issue.ID)
	if updated.Status != "in_progress" || updated.ExecutionPhase != "waiting_children" {
		t.Fatalf("wakeup restoration overwrote new workflow state: %+v", updated)
	}
}
