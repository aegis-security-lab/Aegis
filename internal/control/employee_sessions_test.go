package control

import (
	"testing"
	"time"
)

func TestWorkingEmployeeCanReceiveIssueInAnotherTask(t *testing.T) {
	store := configuredStore(t)
	first, err := store.CreateIssue(CreateIssueInput{Title: "First assignment", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateIssue(CreateIssueInput{Title: "Parallel task assignment", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002"}); err != nil {
		t.Fatalf("working employee could not receive a different Task: %v", err)
	}
	if _, err = store.CreateIssue(CreateIssueInput{Title: "Parallel employee", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-003"}); err != nil {
		t.Fatalf("different employee should remain available: %v", err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", first.ID).Updates(map[string]any{"status": "done", "completed_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateIssue(CreateIssueInput{Title: "Assignment after completion", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002"}); err != nil {
		t.Fatalf("employee did not become available after task completion: %v", err)
	}
}

func TestPositionIDCannotReplaceEmployeeID(t *testing.T) {
	store := configuredStore(t)
	if _, err := store.CreateIssue(CreateIssueInput{Title: "Invalid position assignment", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "ui-design-engineer"}); err == nil {
		t.Fatal("position id was accepted as an employee assignee")
	}
	if _, err := store.CreateIssue(CreateIssueInput{Title: "Named designer assignment", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "ui-design-engineer-001"}); err != nil {
		t.Fatalf("specific employee assignment failed: %v", err)
	}
}

func TestWakeupWaitsForEmployeesActiveSession(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Current work", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer-002", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	comment := IssueComment{ID: nextID("comment"), IssueID: issue.ID, AuthorType: "operator", AuthorID: "operator", Body: "Please review this after the current turn.", Type: "normal", CreatedAt: now}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID, AgentID: "backend-engineer-002", Reason: "issue_comment_assignee", Status: "queued", CreatedAt: now}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.dispatchWakeup(wakeup.ID)
	if err = store.db.First(&wakeup, "id = ?", wakeup.ID).Error; err != nil {
		t.Fatal(err)
	}
	if wakeup.Status != "queued" || wakeup.ExecutionID != "" {
		t.Fatalf("wakeup was injected into active employee work: %+v", wakeup)
	}
}

func TestEmployeeReusesSessionForIssueAndGetsNewSessionForAnotherIssue(t *testing.T) {
	store := configuredStore(t)
	first, err := store.CreateIssue(CreateIssueInput{Title: "First", Objective: "First objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	firstExecution, err := store.createExecution(first, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Execution{}).Where("id = ?", firstExecution.ID).Updates(map[string]any{"status": "completed", "finished_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", first.ID).Updates(map[string]any{"status": "done", "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIssue(CreateIssueInput{ParentID: first.ID, Title: "Second", Objective: "Second objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	secondExecution, err := store.createExecution(second, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := store.EmployeeSessionForIssue("backend-engineer", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := store.EmployeeSessionForIssue("backend-engineer", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstExecution.SessionID != firstSession.SessionID || secondExecution.SessionID != secondSession.SessionID {
		t.Fatalf("executions did not bind their Issue sessions: first=%+v second=%+v", firstExecution, secondExecution)
	}
	if firstExecution.SessionID == secondExecution.SessionID {
		t.Fatal("different Issues reused one Session")
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", second.ID).Updates(map[string]any{"status": "done", "completed_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Execution{}).Where("id = ?", secondExecution.ID).Updates(map[string]any{"status": "completed", "finished_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateIssue(CreateIssueInput{Title: "Other task", Objective: "Other objective", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	otherExecution, err := store.createExecution(other, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if otherExecution.SessionID == firstSession.SessionID || otherExecution.SessionID == secondSession.SessionID {
		t.Fatal("another Issue reused an existing Issue Session")
	}
}

func TestEmployeesHaveIndependentSessions(t *testing.T) {
	store := configuredStore(t)
	backend, err := store.EmployeeSession("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := store.EmployeeSession("frontend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if backend.SessionID == frontend.SessionID {
		t.Fatal("different employees shared one Pi Session")
	}
}

func TestEmployeeSessionUsesHiddenWorkspaceAndExcludesConcierge(t *testing.T) {
	store := configuredStore(t)
	session, err := store.EmployeeSession("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	home, err := store.GetIssue(session.HomeIssueID)
	if err != nil {
		t.Fatal(err)
	}
	if !home.Hidden || home.AssigneeAgentID != "backend-engineer" {
		t.Fatalf("unexpected employee workspace: %+v", home)
	}
	if _, err = store.EmployeeSession(conciergeAgentID); err == nil {
		t.Fatal("concierge must not own an employee session")
	}
	if _, err = store.EmployeeWorkspace(conciergeAgentID); err == nil {
		t.Fatal("concierge must not appear as an employee workspace")
	}
}

func TestEmployeeWorkspaceScopesConversationToTask(t *testing.T) {
	store := configuredStore(t)
	first, _ := store.CreateIssue(CreateIssueInput{Title: "First", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	a, _ := store.createExecution(first, "backend-engineer", "work")
	completedAt := time.Now().Add(-time.Second)
	_ = store.db.Model(&Execution{}).Where("id = ?", a.ID).Updates(map[string]any{"status": "completed", "finished_at": completedAt}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", first.ID).Updates(map[string]any{"status": "done", "completed_at": completedAt}).Error
	second, _ := store.CreateIssue(CreateIssueInput{Title: "Second", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	b, _ := store.createExecution(second, "backend-engineer", "work")
	now := time.Now()
	for _, message := range []Message{
		{ID: nextID("message"), ExecutionID: a.ID, IssueID: first.ID, Role: "assistant", Content: "first issue", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute)},
		{ID: nextID("message"), ExecutionID: b.ID, IssueID: second.ID, Role: "assistant", Content: "second issue", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	attachment, err := store.captureGeneratedAttachment(second, b.ID, "reports/final.md", "final.md", "Final report", []byte("report"))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.EmployeeWorkspace("backend-engineer", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Messages) != 1 || workspace.Messages[0].Content != "second issue" {
		t.Fatalf("unexpected employee conversation: %+v", workspace.Messages)
	}
	if len(workspace.Attachments) != 1 || workspace.Attachments[0].ID != attachment.ID || workspace.Attachments[0].ExecutionID != b.ID {
		t.Fatalf("published attachment missing from employee conversation: %+v", workspace.Attachments)
	}
}

func TestEmployeeWorkspaceIncludesCompactExecutionEvents(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Inspect", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	event := ExecutionEvent{
		ID:          nextID("event"),
		ExecutionID: execution.ID,
		IssueID:     issue.ID,
		Type:        "tool",
		Title:       "Run command",
		ToolName:    "bash",
		Status:      "completed",
		InputJSON:   `{"command":"go test ./...","description":"Run the test suite"}`,
		OutputJSON:  `{"stdout":"large output that should only be loaded from the detail endpoint"}`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err = store.db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	workspace, err := store.EmployeeWorkspace("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Events) != 1 {
		t.Fatalf("expected one employee event, got %+v", workspace.Events)
	}
	if workspace.Events[0].OutputJSON != "" {
		t.Fatalf("employee event should be compact: %+v", workspace.Events[0])
	}
	if workspace.Events[0].InputJSON == "" {
		t.Fatalf("employee event should retain compact descriptive input: %+v", workspace.Events[0])
	}
}

func TestEmployeeActivityReturnsOnlyChangesAfterWatermark(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Live activity", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	time.Sleep(time.Millisecond)
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("updated_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	message := Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: issue.ID, Role: "assistant", Content: "working", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err = store.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	event := ExecutionEvent{ID: nextID("event"), ExecutionID: execution.ID, IssueID: issue.ID, Type: "tool", Title: "Inspect files", ToolName: "ls", Status: "running", InputJSON: `{"description":"Inspect workspace files","path":"."}`, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err = store.db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	attachment, err := store.captureGeneratedAttachment(issue, execution.ID, "reports/live.md", "live.md", "Live report", []byte("report"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}

	activity, err := store.EmployeeActivity("backend-engineer", before)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Executions) != 1 || activity.Executions[0].Status != "running" {
		t.Fatalf("unexpected execution delta: %+v", activity.Executions)
	}
	if len(activity.Messages) != 1 || activity.Messages[0].Content != "working" {
		t.Fatalf("unexpected message delta: %+v", activity.Messages)
	}
	if len(activity.Events) != 1 || activity.Events[0].OutputJSON != "" {
		t.Fatalf("unexpected compact event delta: %+v", activity.Events)
	}
	if len(activity.Attachments) != 1 || activity.Attachments[0].ID != attachment.ID {
		t.Fatalf("unexpected attachment delta: %+v", activity.Attachments)
	}
	if len(activity.Issues) != 1 || activity.Issues[0].ID != issue.ID {
		t.Fatalf("unexpected issue delta: %+v", activity.Issues)
	}

	next, err := store.EmployeeActivity("backend-engineer", activity.Watermark)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Executions)+len(next.Messages)+len(next.Events)+len(next.Attachments)+len(next.Issues) != 0 {
		t.Fatalf("unchanged activity leaked past watermark: %+v", next)
	}
}
