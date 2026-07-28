package control

import (
	"io"
	"os/exec"
	"testing"
	"time"
)

type trackedWriteCloser struct {
	closed bool
}

func (w *trackedWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (w *trackedWriteCloser) Close() error {
	w.closed = true
	return nil
}

var _ io.WriteCloser = (*trackedWriteCloser)(nil)

func TestCancelTaskStopsEntireUnfinishedTree(t *testing.T) {
	store := configuredStore(t)
	task, err := store.CreateIssue(CreateIssueInput{Title: "Task", Objective: "Complete the task tree.", Priority: "high", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: task.ID, Title: "Child", Objective: "Complete the child.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateIssue(CreateIssueInput{ParentID: child.ID, Title: "Grandchild", Objective: "Complete the grandchild.", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	doneChild, err := store.CreateIssue(CreateIssueInput{ParentID: task.ID, Title: "Already done", Objective: "Remain complete.", Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002"})
	if err != nil {
		t.Fatal(err)
	}
	done := "done"
	if _, err = store.UpdateIssue(doneChild.ID, UpdateIssueInput{Status: &done}); err != nil {
		t.Fatal(err)
	}

	activeExecution, err := store.createExecution(child, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(activeExecution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	queuedExecution, err := store.createExecution(grandchild, "frontend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	completedExecution, err := store.createExecution(doneChild, "backend-engineer-002", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.updateExecution(completedExecution.ID, map[string]any{"status": "completed", "finished_at": &now}); err != nil {
		t.Fatal(err)
	}
	approval := Approval{ID: nextID("approval"), ExecutionID: activeExecution.ID, IssueID: child.ID, Status: "pending", CreatedAt: now}
	if err = store.db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: grandchild.ID, AgentID: "red-team-engineer", Status: "queued", CreatedAt: now}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	stdin := &trackedWriteCloser{}
	session := &PiSession{
		manager: manager, key: activeExecution.ID, executionID: activeExecution.ID,
		issueID: child.ID, agentID: "backend-engineer", kind: "work", stdin: stdin, cmd: &exec.Cmd{},
	}
	manager.sessions[activeExecution.ID] = session

	result, err := manager.CancelTask(task.ID, "operator cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalIssues != 4 || result.CancelledIssues != 3 || result.CancelledExecutions != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !session.closed.Load() || !stdin.closed {
		t.Fatal("active Pi session was not closed")
	}

	for _, id := range []string{task.ID, child.ID, grandchild.ID} {
		issue, getErr := store.GetIssue(id)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if issue.Status != "cancelled" || issue.ExecutionPhase != "completed" || issue.CancelledAt == nil {
			t.Fatalf("issue not cancelled: %+v", issue)
		}
	}
	preserved, _ := store.GetIssue(doneChild.ID)
	if preserved.Status != "done" {
		t.Fatalf("completed history changed: %+v", preserved)
	}
	for _, id := range []string{activeExecution.ID, queuedExecution.ID} {
		var execution Execution
		if err = store.db.First(&execution, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if execution.Status != "cancelled" || execution.FinishedAt == nil {
			t.Fatalf("execution not cancelled: %+v", execution)
		}
	}
	var preservedExecution Execution
	_ = store.db.First(&preservedExecution, "id = ?", completedExecution.ID).Error
	if preservedExecution.Status != "completed" {
		t.Fatalf("completed execution changed: %+v", preservedExecution)
	}
	var approvalCount int64
	if err = store.db.Model(&Approval{}).Where("id = ?", approval.ID).Count(&approvalCount).Error; err != nil || approvalCount != 0 {
		t.Fatalf("invalid approval was not removed: err=%v count=%d", err, approvalCount)
	}
	if result.RemovedApprovals != 1 {
		t.Fatalf("removed approval count = %d, want 1", result.RemovedApprovals)
	}
	_ = store.db.First(&wakeup, "id = ?", wakeup.ID).Error
	if wakeup.Status != "cancelled" || wakeup.CompletedAt == nil {
		t.Fatalf("wakeup not cancelled: %+v", wakeup)
	}

	todo := "todo"
	if _, err = store.UpdateIssue(child.ID, UpdateIssueInput{Status: &todo}); err == nil {
		t.Fatal("cancelled task child should not be reopenable")
	}
	if _, err = manager.CancelTask(child.ID, "invalid"); err == nil {
		t.Fatal("child Issue must not be accepted as a task")
	}
	second, err := manager.CancelTask(task.ID, "idempotent retry")
	if err != nil {
		t.Fatal(err)
	}
	if second.CancelledIssues != 0 || second.CancelledExecutions != 0 {
		t.Fatalf("idempotent retry changed terminal records: %+v", second)
	}
}
