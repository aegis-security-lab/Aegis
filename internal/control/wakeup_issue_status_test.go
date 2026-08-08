package control

import (
	"strings"
	"testing"
	"time"
)

func TestWakeupExecutionMakesIssueActiveThenRestoresPriorTerminalState(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Completed discussion", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
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
	root, _ := store.taskRoot(issue)
	identity, _ := store.taskAgent(root.ID, issue.AssigneeTaskAgentID)
	if identity.Status != "active" {
		t.Fatalf("wakeup did not reactivate TaskAgent: %+v", identity)
	}

	manager.restoreIssueAfterWakeup(issue.ID, "execution-wakeup")
	restored, _ := store.GetIssue(issue.ID)
	if restored.Status != "done" || restored.ExecutionPhase != "completed" || restored.CheckoutExecutionID != "" {
		t.Fatalf("pure wakeup did not restore prior terminal state: %+v", restored)
	}
}

func TestWakeupPromptRequiresVisibleReplyOnCurrentIssue(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Follow-up", Objective: "Answer the operator.", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorID: "operator", Body: "请说明上次结果中的关键步骤。"}
	if err := store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: issue.AssigneeAgentID, Reason: "issue_comment_assignee", Status: "queued"}
	prompt, err := wakeupPrompt(issue, wakeup, store.db)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"same Issue", "MUST explicitly call aegis_board", "action=comment", "visible as a reply"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("wakeup prompt missing %q: %s", required, prompt)
		}
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
