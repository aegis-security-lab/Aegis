package control

import (
	"strings"
	"testing"
	"time"
)

func TestAgentRootIssueUsesCurrentWorkspaceAndCreator(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "agent root", Objective: "audit", Status: "backlog",
		Priority: "medium", WorkMode: "autonomous", CreatedBy: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issue.CreatedBy != "red-team-lead" || issue.Workspace != TaskWorkspacePath {
		t.Fatalf("agent root issue=%+v workspace=%q", issue, issue.Workspace)
	}
}

func TestDeleteIssueTreeRemovesDescendantsAndRejectsActiveWork(t *testing.T) {
	store := configuredStore(t)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}

	_, root, err := store.CreateTask(CreateIssueInput{
		Title: "root", Objective: "root objective", Status: "backlog",
		Priority: "medium", WorkMode: "autonomous", Workspace: store.Config().Workspace, AssigneeAgentID: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{
		ParentID: root.ID, Title: "child", Objective: "child objective",
		Status: "backlog", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(child, "red-team-lead", "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.DeleteBoardIssue(root.ID); err == nil || !strings.Contains(err.Error(), "仍在执行") {
		t.Fatalf("active deletion err=%v", err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{
		"status": "completed", "finished_at": time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := manager.DeleteBoardIssue(root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedIssues != 2 || result.DeletedExecutions != 1 || result.DeletedTasks != 1 {
		t.Fatalf("delete result=%+v", result)
	}
	for _, id := range []string{root.ID, child.ID} {
		if _, getErr := store.GetIssue(id); getErr == nil {
			t.Fatalf("issue %s still exists", id)
		}
	}
}
