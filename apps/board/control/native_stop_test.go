package control

import "testing"

type trackedExecutionAborter struct {
	executionIDs []string
}

func (a *trackedExecutionAborter) AbortIssue(string) {}
func (a *trackedExecutionAborter) AbortExecution(executionID string) bool {
	a.executionIDs = append(a.executionIDs, executionID)
	return true
}

func TestStopExecutionAbortsNativeRuntimeAndFencesCompletion(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Stop native runtime", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running", "runtime_type": "agentcore"}); err != nil {
		t.Fatal(err)
	}
	aborter := &trackedExecutionAborter{}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}, nativeSessions: aborter}
	if err = manager.StopExecution(execution.ID); err != nil {
		t.Fatal(err)
	}
	if len(aborter.executionIDs) != 1 || aborter.executionIDs[0] != execution.ID {
		t.Fatalf("native aborts=%v", aborter.executionIDs)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	var stopped Execution
	if err = store.db.First(&stopped, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "stopped" {
		t.Fatalf("late completion overwrote stopped status: %+v", stopped)
	}
}
