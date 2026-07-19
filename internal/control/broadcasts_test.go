package control

import (
	"bytes"
	"strings"
	"testing"
)

type recordingWriteCloser struct {
	bytes.Buffer
}

func (w *recordingWriteCloser) Close() error { return nil }

func TestTaskBroadcastPersistsAndDeliversOnlyWithinTaskTree(t *testing.T) {
	store := configuredStore(t)
	task, err := store.CreateIssue(CreateIssueInput{
		Title: "Coordinate implementation", Objective: "Complete coordinated work.", Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceIssue, err := store.CreateIssue(CreateIssueInput{
		ParentID: task.ID, Title: "Inspect API", Objective: "Inspect the API.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	targetIssue, err := store.CreateIssue(CreateIssueInput{
		ParentID: task.ID, Title: "Implement UI", Objective: "Implement the UI.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := store.CreateIssue(CreateIssueInput{
		Title: "Unrelated task", Objective: "Remain isolated.", Priority: "low", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceExecution, _ := store.createExecution(sourceIssue, "backend-engineer", "work")
	targetExecution, _ := store.createExecution(targetIssue, "frontend-engineer", "work")
	otherExecution, _ := store.createExecution(otherTask, "backend-engineer", "work")
	validationExecution, _, err := store.createInternalExecution(targetIssue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	targetWriter := &recordingWriteCloser{}
	otherWriter := &recordingWriteCloser{}
	validatorWriter := &recordingWriteCloser{}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[sourceExecution.ID] = &PiSession{
		executionID: sourceExecution.ID, issueID: sourceIssue.ID, agentID: sourceExecution.AgentID,
		kind: "work", controlToken: "source-broadcast-token",
	}
	manager.sessions[targetExecution.ID] = &PiSession{
		executionID: targetExecution.ID, issueID: targetIssue.ID, agentID: targetExecution.AgentID,
		kind: "work", controlToken: "target-broadcast-token", stdin: targetWriter,
	}
	manager.sessions[otherExecution.ID] = &PiSession{
		executionID: otherExecution.ID, issueID: otherTask.ID, agentID: otherExecution.AgentID,
		kind: "work", controlToken: "other-broadcast-token", stdin: otherWriter,
	}
	manager.sessions[validationExecution.ID] = &PiSession{
		executionID: validationExecution.ID, issueID: targetIssue.ID, agentID: validationExecution.AgentID,
		kind: "validation", controlToken: "validator-token", stdin: validatorWriter,
	}

	if _, err = manager.BroadcastExecution(sourceExecution.ID, "wrong-broadcast-token", BroadcastMessageInput{
		Subject: "Shared contract", Message: "The API returns cursor metadata.", Importance: "important",
	}); err == nil {
		t.Fatal("expected an invalid broadcast token to be rejected")
	}
	broadcast, err := manager.BroadcastExecution(sourceExecution.ID, "source-broadcast-token", BroadcastMessageInput{
		Subject:    " Shared contract ",
		Message:    " The API response includes `nextCursor`; UI pagination should preserve it. ",
		Importance: "important",
	})
	if err != nil {
		t.Fatal(err)
	}
	if broadcast.TaskID != task.ID || broadcast.DeliveredCount != 1 || len(broadcast.RecipientExecutionIDs) != 1 || broadcast.RecipientExecutionIDs[0] != targetExecution.ID {
		t.Fatalf("unexpected broadcast delivery: %+v", broadcast)
	}
	if !strings.Contains(targetWriter.String(), "nextCursor") {
		t.Fatalf("target session did not receive broadcast: %s", targetWriter.String())
	}
	if otherWriter.Len() != 0 || validatorWriter.Len() != 0 {
		t.Fatalf("broadcast escaped its task or reached validator: other=%q validator=%q", otherWriter.String(), validatorWriter.String())
	}

	history, err := manager.ExecutionBroadcastHistory(targetExecution.ID, "target-broadcast-token", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != broadcast.ID || len(history[0].RecipientExecutionIDs) != 1 {
		t.Fatalf("broadcast history=%+v", history)
	}
	otherHistory, err := manager.ExecutionBroadcastHistory(otherExecution.ID, "other-broadcast-token", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherHistory) != 0 {
		t.Fatalf("unrelated task saw broadcasts: %+v", otherHistory)
	}
	detail, err := store.GetIssueDetail(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Broadcasts) != 1 || detail.Broadcasts[0].Subject != "Shared contract" {
		t.Fatalf("task detail broadcasts=%+v", detail.Broadcasts)
	}
}

func TestBroadcastInputAndCoordinationPrompt(t *testing.T) {
	store := configuredStore(t)
	manager := &Manager{store: store, sessions: map[string]*PiSession{
		"execution-1": {executionID: "execution-1", issueID: "missing-issue", controlToken: "broadcast-secret"},
	}}
	if _, err := manager.BroadcastExecution("execution-1", "broadcast-secret", BroadcastMessageInput{}); err == nil {
		t.Fatal("expected an empty broadcast to be rejected")
	}
	prompt := agentBroadcastSystemPrompt("You are a Worker.")
	for _, required := range []string{"aegis_broadcast", "aegis_list_broadcasts", "untrusted context", "does not transfer ownership"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("broadcast prompt missing %q: %s", required, prompt)
		}
	}
}
