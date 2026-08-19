package control

import (
	"strings"
	"testing"
	"time"
)

func TestAgentRootIssueUsesCurrentWorkspaceAndCreator(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "agent root", Objective: "audit", Status: "todo",
		Priority: "middle", WorkMode: "autonomous", CreatedBy: "red-team-lead",
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
		Title: "root", Objective: "root objective", Status: "todo",
		Priority: "middle", WorkMode: "autonomous", Workspace: store.Config().Workspace, AssigneeAgentID: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{
		ParentID: root.ID, Title: "child", Objective: "child objective",
		Status: "todo", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead",
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

func TestDeleteRootIssuePreservesTaskUntilLastRootIsDeleted(t *testing.T) {
	store := configuredStore(t)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	task, firstRoot, err := store.CreateTask(CreateIssueInput{
		Title: "multi-root task", Objective: "Keep the Task while any root remains.", Status: "todo",
		Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "red-team-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := store.CreateIssue(CreateIssueInput{
		TaskSourceID: task.ID, Title: "second root", Objective: "Remain after the first root is deleted.",
		Status: "todo", Priority: "middle", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := manager.DeleteBoardIssue(firstRoot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedIssues != 1 || result.DeletedTasks != 0 {
		t.Fatalf("first root deletion removed Task: %+v", result)
	}
	if _, err = store.GetTask(task.ID); err != nil {
		t.Fatalf("Task disappeared while another root remained: %v", err)
	}
	if _, err = store.GetIssue(secondRoot.ID); err != nil {
		t.Fatalf("remaining root disappeared: %v", err)
	}

	result, err = manager.DeleteBoardIssue(secondRoot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedIssues != 1 || result.DeletedTasks != 1 {
		t.Fatalf("last root deletion did not remove Task: %+v", result)
	}
	if _, err = store.GetTask(task.ID); err == nil {
		t.Fatal("Task still exists after its final root was deleted")
	}
}
