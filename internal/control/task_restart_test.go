package control

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRestartTaskCreatesNewRootIssueWithOriginalSource(t *testing.T) {
	store := configuredStore(t)
	task, original, err := store.CreateTask(CreateIssueInput{
		Title: "Repeatable security review", Description: "Inspect the target.", Objective: "Produce evidence.",
		Priority: "critical", WorkMode: "guided",
		Workspace: store.Config().Workspace, Context: "Original context", Constraints: "Original boundary",
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
	if restarted.Title != original.Title || restarted.Description != original.Description || restarted.Objective != original.Objective || restarted.Priority != original.Priority || restarted.WorkMode != original.WorkMode || restarted.AssigneeAgentID != original.AssigneeAgentID || restarted.Workspace != original.Workspace || restarted.Context != original.Context || restarted.Constraints != original.Constraints {
		t.Fatalf("restart did not preserve task parameters: original=%+v restarted=%+v", original, restarted)
	}

	again, err := manager.RestartTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.TaskSourceID != task.ID {
		t.Fatalf("repeated restart source=%q, want Task %q", again.TaskSourceID, task.ID)
	}
}

func TestTaskWorkspaceListsNestedFiles(t *testing.T) {
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
	issue, err := store.CreateIssue(CreateIssueInput{Title: "List workspace", Priority: "medium", WorkMode: "guided", Workspace: root})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.TaskWorkspace(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(workspace.Entries))
	for _, entry := range workspace.Entries {
		paths = append(paths, entry.Path)
	}
	for _, expected := range []string{"README.md", "nested", "nested/report.txt"} {
		if !slices.Contains(paths, expected) {
			t.Fatalf("workspace entries %v missing %q", paths, expected)
		}
	}
}

func TestStartupMigratesLegacyRootIssueToReusableTaskOnce(t *testing.T) {
	store := configuredStore(t)
	legacy, err := store.CreateIssue(CreateIssueInput{
		Title: "Legacy root task", Priority: "high", WorkMode: "guided", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.TaskSourceID != "" {
		t.Fatalf("test fixture unexpectedly has a Task source: %+v", legacy)
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.db.First(&legacy, "id = ?", legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.TaskSourceID == "" {
		t.Fatal("legacy root Issue was not assigned a reusable Task")
	}
	var count int64
	if err = reopened.db.Model(&Task{}).Where("id = ?", legacy.TaskSourceID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("migrated Task count=%d err=%v", count, err)
	}

	reopenedAgain, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = reopenedAgain.db.Model(&Task{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("Task migration duplicated records: count=%d err=%v", count, err)
	}
}
