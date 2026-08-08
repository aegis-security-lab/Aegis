package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestartTaskCreatesNewRootIssueWithOriginalSource(t *testing.T) {
	store := configuredStore(t)
	task, original, err := store.CreateTask(CreateIssueInput{
		Title: "Repeatable security review", Description: "Inspect the target.", Objective: "Produce evidence.",
		Priority: "high", WorkMode: "guided",
		Workspace: store.Config().Workspace, Constraints: "Original boundary",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	restarted, err := manager.RestartTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.ID == original.ID || restarted.ParentID != "" || restarted.TaskSourceID != task.ID {
		t.Fatalf("restart did not create a sourced root Issue: original=%+v restarted=%+v", original, restarted)
	}
	if restarted.Title != original.Title || restarted.Description != original.Description || restarted.Objective != original.Objective || restarted.Priority != original.Priority || restarted.WorkMode != original.WorkMode || restarted.AssigneeAgentID != original.AssigneeAgentID || restarted.Workspace != original.Workspace || restarted.Constraints != original.Constraints {
		t.Fatalf("restart did not preserve task parameters: original=%+v restarted=%+v", original, restarted)
	}

	again, err := manager.RestartTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.TaskSourceID != task.ID {
		t.Fatalf("repeated restart source=%q, want Task %q", again.TaskSourceID, task.ID)
	}
	if _, err = manager.RestartTask(original.ID); err == nil {
		t.Fatal("RestartTask accepted an Issue ID instead of a Task ID")
	}
}

func TestCreateRootIssueInExistingTaskKeepsTaskBinding(t *testing.T) {
	store := configuredStore(t)
	task, original, err := store.CreateTask(CreateIssueInput{
		Title: "Multi-root review", Objective: "Coordinate independent roots.",
		Priority: "middle", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateIssue(CreateIssueInput{
		TaskSourceID: task.ID, Title: "Manual root Issue", Objective: "Run independently.",
		Priority: "high", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ParentID != "" || created.TaskSourceID != task.ID {
		t.Fatalf("manual Issue was not a task-bound root: %+v", created)
	}
	if created.ContainerID != original.ContainerID || created.ContainerProfileID != original.ContainerProfileID || created.Workspace != original.Workspace {
		t.Fatalf("manual root did not inherit task runtime: original=%+v created=%+v", original, created)
	}

	var taskCount int64
	if err = store.db.Model(&Task{}).Count(&taskCount).Error; err != nil || taskCount != 1 {
		t.Fatalf("manual root created another Task: count=%d err=%v", taskCount, err)
	}
	var rootCount int64
	if err = store.db.Model(&Issue{}).Where("task_source_id = ? AND parent_id = ''", task.ID).Count(&rootCount).Error; err != nil || rootCount != 2 {
		t.Fatalf("task root count=%d err=%v, want 2", rootCount, err)
	}
}

func TestUpdateTaskBudgetUpdatesEveryRootIssue(t *testing.T) {
	store := configuredStore(t)
	task, firstRoot, err := store.CreateTask(CreateIssueInput{
		Title: "Shared budget", Objective: "Apply one Task budget to every root.", Priority: "middle", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := store.CreateIssue(CreateIssueInput{
		TaskSourceID: task.ID, Title: "Additional root", Objective: "Share the Task budget.", Priority: "middle", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	minutes := 45
	updated, err := store.UpdateTaskBudget(task.ID, &minutes)
	if err != nil || updated.TimeBudgetMinutes == nil || *updated.TimeBudgetMinutes != minutes {
		t.Fatalf("updated Task=%+v err=%v", updated, err)
	}
	for _, rootID := range []string{firstRoot.ID, secondRoot.ID} {
		root, getErr := store.GetIssue(rootID)
		if getErr != nil || root.TimeBudgetMinutes == nil || *root.TimeBudgetMinutes != minutes {
			t.Fatalf("root %s budget=%v err=%v", rootID, root.TimeBudgetMinutes, getErr)
		}
	}
	if _, err = store.UpdateTaskBudget(firstRoot.ID, &minutes); err == nil {
		t.Fatal("UpdateTaskBudget accepted an Issue id instead of a Task id")
	}
}

func TestFreshTaskWorkspaceDoesNotExposeHostFiles(t *testing.T) {
	store := configuredStore(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "report.txt"), []byte("report"), 0o600); err != nil {
		t.Fatal(err)
	}
	issue, err := store.CreateIssue(CreateIssueInput{Title: "List workspace", Priority: "middle", WorkMode: "guided", Workspace: root})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.TaskWorkspace(issue.TaskSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Root != "/workspace" || len(workspace.Entries) != 0 {
		t.Fatalf("fresh task exposed host workspace files: %+v", workspace)
	}
}
