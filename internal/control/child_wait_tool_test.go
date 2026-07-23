package control

import "testing"

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

	wait, err := manager.WaitForChildIssues(execution.ID, "secret", WaitForChildIssuesInput{ChildIssueIDs: []string{first.ID}})
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

	children := []Issue{{ID: first.ID, Status: "done"}, {ID: second.ID, Status: "in_progress"}}
	if !childWaitConditionSatisfied(wait, children) {
		t.Fatal("selected-child wait was not satisfied when its selected child completed")
	}
	if childWaitConditionSatisfied(IssueChildWait{WaitForAll: true}, children) {
		t.Fatal("wait-for-all was satisfied while another child was active")
	}
	children[1].Status = "cancelled"
	if !childWaitConditionSatisfied(IssueChildWait{WaitForAll: true}, children) {
		t.Fatal("wait-for-all was not satisfied after every child became terminal")
	}
	children[1].Status = "failed"
	if !childWaitConditionSatisfied(IssueChildWait{WaitForAll: true}, children) {
		t.Fatal("wait-for-all was not satisfied after a child failed terminally")
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
		{WaitForAll: true, ChildIssueIDs: []string{"issue-x"}},
		{ChildIssueIDs: []string{"issue-x"}},
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

	wait, err := manager.WaitForChildIssues(execution.ID, "secret", WaitForChildIssuesInput{WakeupIDs: []string{wakeup.ID}})
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
