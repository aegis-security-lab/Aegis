package control

import (
	"strings"
	"testing"
)

func TestSessionExitDuringShutdownRemainsRecoverable(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Recover worker after service restart", Objective: "Resume the interrupted worker.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CheckoutIssue(issue.ID, CheckoutIssueInput{
		AgentID: execution.AgentID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{manager: manager, key: execution.ID, executionID: execution.ID, issueID: issue.ID, kind: "work"}
	manager.sessions[session.key] = session
	manager.BeginShutdown()
	manager.sessionExited(session, nil)

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "in_progress" || updated.ExecutionPhase != "active" || updated.CheckoutExecutionID != execution.ID {
		t.Fatalf("shutdown exit made Issue unrecoverable: %+v", updated)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "running" {
		t.Fatalf("shutdown exit changed execution status: %+v", execution)
	}
}

func TestClosePreviousRuntimeClosesSameExecutionRestart(t *testing.T) {
	manager := &Manager{sessions: map[string]*PiSession{}}
	stdin := &trackedWriteCloser{}
	previous := &PiSession{
		manager: manager, key: "execution-1", executionID: "execution-1",
		sessionID: "session-1", stdin: stdin,
	}
	manager.sessions[previous.key] = previous

	manager.closePreviousRuntimeForSession("session-1", "execution-1")

	if !previous.closed.Load() || !stdin.closed {
		t.Fatal("same-execution restart did not close the previous Pi runtime")
	}
	if manager.sessions[previous.key] != nil {
		t.Fatal("closed previous Pi runtime remained registered")
	}
}

func TestContainerExecutionCleanupTargetsExactExecutionEnvironment(t *testing.T) {
	for _, expected := range []string{"AEGIS_EXECUTION_ID=$1", "/proc/[0-9]*/environ", "grep -Fqx", `kill -"$2"`} {
		if !strings.Contains(terminateContainerExecutionScript, expected) {
			t.Fatalf("container cleanup script does not contain %q", expected)
		}
	}
	if strings.Contains(terminateContainerExecutionScript, "pkill") {
		t.Fatal("container cleanup must not rely on the Pi process title")
	}
}

func TestCloseRuntimeRemovesAndClosesRegisteredSession(t *testing.T) {
	manager := &Manager{sessions: map[string]*PiSession{}}
	stdin := &trackedWriteCloser{}
	session := &PiSession{manager: manager, key: "execution-1", executionID: "execution-1", stdin: stdin}
	manager.sessions[session.key] = session

	manager.closeRuntime(session.executionID)

	if !session.closed.Load() || !stdin.closed {
		t.Fatal("runtime was not closed")
	}
	if manager.sessions[session.key] != nil {
		t.Fatal("closed runtime remained registered")
	}
}
