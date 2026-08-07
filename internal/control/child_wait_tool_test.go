package control

import (
	"testing"
	"time"
)

func TestWaitForChildIssuesPersistsSelectionAndReleasesParent(t *testing.T) {
	store := configuredStore(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "First", Priority: "high", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Second", Priority: "medium", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(parent, "backend-engineer", "continuation")
	if err != nil {
		t.Fatal(err)
	}
	parent, err = store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"}})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{key: execution.ID, executionID: execution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}

	wait, err := manager.WaitForChildIssues(execution.ID, "secret", WaitForChildIssuesInput{ChildIssueIDs: []string{first.ID}, EstimatedWaitMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	if wait.WaitForAll || len(wait.ChildIssueIDs) != 1 || wait.ChildIssueIDs[0] != first.ID || wait.Status != "waiting" {
		t.Fatalf("unexpected wait: %+v", wait)
	}
	updated, err := store.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ExecutionPhase != "waiting_children" || updated.CheckoutExecutionID != "" || updated.CurrentExecutionID != execution.ID {
		t.Fatalf("parent was not released into waiting_children: %+v", updated)
	}

	completedAt := wait.CreatedAt.Add(time.Second)
	children := []Issue{{ID: first.ID, Status: "done", UpdatedAt: completedAt}, {ID: second.ID, Status: "in_progress", UpdatedAt: completedAt}}
	if !childWaitConditionSatisfied(wait, children) {
		t.Fatal("selected-child wait was not satisfied when its selected child completed")
	}
	allWait := IssueChildWait{WaitForAll: true, CreatedAt: wait.CreatedAt}
	if !childWaitConditionSatisfied(allWait, children) {
		t.Fatal("monitor-all was not satisfied when one child newly completed")
	}
	if childWaitConditionSatisfied(IssueChildWait{WaitForAll: true, CreatedAt: completedAt.Add(time.Second)}, children) {
		t.Fatal("an old completion triggered a later wait again")
	}
	children[1].Status = "done"
	children[1].Labels = []string{"failed"}
	children[1].UpdatedAt = completedAt.Add(2 * time.Second)
	if !childWaitConditionSatisfied(IssueChildWait{WaitForAll: true, CreatedAt: completedAt.Add(time.Second)}, children) {
		t.Fatal("a subsequent child completion did not trigger the next wait")
	}
}

func TestWaitForChildIssuesRejectsInvalidSelection(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	_, _ = store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Priority: "medium", WorkMode: "autonomous"})
	execution, _ := store.createExecution(parent, "backend-engineer", "work")
	_, _ = store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"}})
	manager := &Manager{store: store, sessions: map[string]*PiSession{execution.ID: {key: execution.ID, executionID: execution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}}}
	for _, input := range []WaitForChildIssuesInput{
		{},
		{WaitForAll: true, ChildIssueIDs: []string{"issue-x"}, EstimatedWaitMinutes: 5},
		{ChildIssueIDs: []string{"issue-x"}, EstimatedWaitMinutes: 5},
	} {
		if _, err := manager.WaitForChildIssues(execution.ID, "secret", input); err == nil {
			t.Fatalf("invalid wait input was accepted: %+v", input)
		}
	}
}

func TestWaitForChildIssuesCanWaitForExactCommentWakeup(t *testing.T) {
	store := configuredStore(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, _ := store.createExecution(parent, "backend-engineer", "continuation")
	parent, _ = store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: "backend-engineer", ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"}})
	comment := IssueComment{ID: nextID("comment"), IssueID: child.ID, ExecutionID: execution.ID, AuthorType: "agent", AuthorID: "backend-engineer", Body: "Please reply.", CreatedAt: parent.CreatedAt}
	if err := store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: child.ID, CommentID: comment.ID, AgentID: "frontend-engineer", Status: "queued", CreatedAt: parent.CreatedAt}
	if err := store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{execution.ID: {key: execution.ID, executionID: execution.ID, issueID: parent.ID, agentID: "backend-engineer", controlToken: "secret"}}}

	wait, err := manager.WaitForChildIssues(execution.ID, "secret", WaitForChildIssuesInput{WakeupIDs: []string{wakeup.ID}, EstimatedWaitMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(wait.WakeupIDs) != 1 || wait.WakeupIDs[0] != wakeup.ID {
		t.Fatalf("unexpected wait: %+v", wait)
	}
	if manager.childWaitConditionSatisfied(wait, []Issue{child}) {
		t.Fatal("queued wakeup satisfied exact-response wait")
	}
	if err = store.db.Model(&AgentWakeup{}).Where("id = ?", wakeup.ID).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	if !manager.childWaitConditionSatisfied(wait, []Issue{child}) {
		t.Fatal("completed wakeup did not satisfy exact-response wait")
	}
}
