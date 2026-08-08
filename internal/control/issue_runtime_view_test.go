package control

import (
	"testing"
	"time"
)

func TestIssueRuntimeRecoveryDoesNotClaimDisconnectedExecutionIsRunning(t *testing.T) {
	now := time.Now()
	execution := Execution{ID: "execution-old", Status: "disconnected", UpdatedAt: now}
	view := resolveIssueRuntimeView(Issue{
		ID:                  "issue-1",
		Status:              "todo",
		ExecutionPhase:      "recovering",
		CurrentExecutionID:  execution.ID,
		RecoveryExecutionID: execution.ID,
		UpdatedAt:           now,
	}, issueRuntimeFacts{currentExecution: &execution})

	if view.State != "recovery_pending" || view.Kind != issueRuntimeKindWaiting || view.Health != "waiting" {
		t.Fatalf("runtime=%+v, want a waiting recovery projection", view)
	}
}

func TestIssueRuntimeDetectsStaleWaitingChildrenPhase(t *testing.T) {
	view := resolveIssueRuntimeView(Issue{
		ID:             "issue-1",
		Status:         "in_progress",
		ExecutionPhase: "waiting_children",
		UpdatedAt:      time.Now(),
	}, issueRuntimeFacts{})

	if view.State != "wait_stalled" || view.Kind != issueRuntimeKindFailed || view.Health != "stalled" {
		t.Fatalf("runtime=%+v, want a stalled wait projection", view)
	}
}

func TestStateRuntimeUsesCurrentExecutionPointerInsteadOfNewestExecution(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Use exact execution pointer", Objective: "Render the durable current execution.",
		Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	current := Execution{
		ID: "execution-current", IssueID: issue.ID, AgentID: "backend-engineer", Kind: "work",
		Status: "running", StartedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	newer := Execution{
		ID: "execution-newer-but-not-current", IssueID: issue.ID, AgentID: "backend-engineer", Kind: "wakeup",
		Status: "disconnected", StartedAt: now, UpdatedAt: now,
	}
	if err = store.db.Create(&[]Execution{current, newer}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "current_execution_id": current.ID,
		"updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	state := store.State()
	var runtime *IssueRuntimeView
	for index := range state.IssueRuntimes {
		if state.IssueRuntimes[index].IssueID == issue.ID {
			runtime = &state.IssueRuntimes[index]
			break
		}
	}
	if runtime == nil {
		t.Fatal("state omitted the Issue runtime projection")
	}
	if runtime.State != "running" || runtime.CurrentExecutionID != current.ID {
		t.Fatalf("runtime=%+v, want the execution selected by currentExecutionId", *runtime)
	}
}
