package control

import (
	"testing"
	"time"
)

func TestIssueAssignmentCreatesAndReusesAgentSession(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Bound work", Objective: "Complete it.", Priority: "medium",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	var binding IssueAgentSession
	if err = store.db.Where("issue_id = ? AND agent_id = ? AND status = ?", issue.ID, "backend-engineer", "active").First(&binding).Error; err != nil {
		t.Fatalf("assignment did not create a binding: %v", err)
	}
	work, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := store.createExecution(issue, "backend-engineer", "continuation")
	if err != nil {
		t.Fatal(err)
	}
	if work.SessionID != binding.SessionID || continuation.SessionID != binding.SessionID {
		t.Fatalf("executions did not reuse binding: binding=%s work=%s continuation=%s", binding.SessionID, work.SessionID, continuation.SessionID)
	}
	if work.IssueAgentSessionID != binding.ID || continuation.IssueAgentSessionID != binding.ID {
		t.Fatalf("executions did not reference binding: work=%+v continuation=%+v", work, continuation)
	}
	if sessions := store.Sessions(); len(sessions) != 1 || sessions[0].Execution.SessionID != binding.SessionID {
		t.Fatalf("Pi Session list was not grouped by durable Session: %+v", sessions)
	}
	now := time.Now()
	for _, message := range []Message{
		{ID: nextID("message"), ExecutionID: work.ID, IssueID: issue.ID, Role: "assistant", Content: "first turn", CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute)},
		{ID: nextID("message"), ExecutionID: continuation.ID, IssueID: issue.ID, Role: "assistant", Content: "continued turn", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	detail, err := store.GetSession(continuation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 2 || detail.Messages[0].Content != "first turn" || detail.Messages[1].Content != "continued turn" {
		t.Fatalf("Session detail did not aggregate execution turns: %+v", detail.Messages)
	}
}

func TestIssueKeepsSeparateSessionsPerAgent(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Shared review", Objective: "Complete it.", Priority: "medium",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	backend, err := store.ensureIssueAgentSession(issue, "backend-engineer", "")
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := store.ensureIssueAgentSession(issue, "frontend-engineer", "")
	if err != nil {
		t.Fatal(err)
	}
	if backend.SessionID == frontend.SessionID {
		t.Fatal("different agents shared one Pi Session")
	}
	other, err := store.CreateIssue(CreateIssueInput{Title: "Other", Objective: "Complete it.", Priority: "medium", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ensureIssueAgentSession(other, "frontend-engineer", backend.SessionID); err == nil {
		t.Fatal("a Pi Session was rebound to another Issue or Agent")
	}
}

func TestRuntimeChangeCreatesNextSessionGeneration(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Move runtime", Objective: "Complete it.", Priority: "medium",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.ensureIssueAgentSession(issue, "backend-engineer", "")
	if err != nil {
		t.Fatal(err)
	}
	issue.ContainerProfileID = "container-new"
	second, err := store.ensureIssueAgentSession(issue, "backend-engineer", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID == second.SessionID || second.Generation != first.Generation+1 {
		t.Fatalf("runtime change did not rotate the Session generation: first=%+v second=%+v", first, second)
	}
	if err = store.db.First(&first, "id = ?", first.ID).Error; err != nil || first.Status != "superseded" {
		t.Fatalf("old binding was not superseded: binding=%+v err=%v", first, err)
	}
}

func TestLegacyExecutionsMigrateToOneCurrentBinding(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Legacy", Objective: "Complete it.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Where("issue_id = ?", issue.ID).Delete(&IssueAgentSession{}).Error; err != nil {
		t.Fatal(err)
	}
	older := Execution{ID: nextID("execution"), IssueID: issue.ID, AgentID: "backend-engineer", Kind: "work", Status: "completed", SessionID: "pi-session-legacy-old", StartedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now()}
	newer := Execution{ID: nextID("execution"), IssueID: issue.ID, AgentID: "backend-engineer", Kind: "rework", Status: "completed", SessionID: "pi-session-legacy-new", StartedAt: time.Now(), UpdatedAt: time.Now()}
	if err = store.db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := store.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	var bindings []IssueAgentSession
	if err = reopened.db.Where("issue_id = ? AND agent_id = ?", issue.ID, "backend-engineer").Order("generation asc").Find(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("bindings=%d, want 2: %+v", len(bindings), bindings)
	}
	activeCount := 0
	for _, binding := range bindings {
		if binding.Status == "active" {
			activeCount++
			if binding.SessionID != newer.SessionID {
				t.Fatalf("migration selected %s, want latest %s", binding.SessionID, newer.SessionID)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("active bindings=%d, want 1: %+v", activeCount, bindings)
	}
}
