package control

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installFakeDocker(t *testing.T) (string, string) {
	t.Helper()
	fakeBin := t.TempDir()
	dockerPath := filepath.Join(fakeBin, "docker")
	logPath := filepath.Join(t.TempDir(), "docker.log")
	statePath := filepath.Join(t.TempDir(), "state")
	volumeStatePath := filepath.Join(t.TempDir(), "volume-state")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "$1" in
  inspect)
    if [ -f "$FAKE_DOCKER_STATE" ]; then cat "$FAKE_DOCKER_STATE"; else printf 'no such container\n' >&2; exit 1; fi
    ;;
  version)
    printf '25.0.0\n'
    ;;
  run|start)
    printf 'running\n' > "$FAKE_DOCKER_STATE"
    printf 'fake-container-id\n'
    ;;
  stop)
    printf 'exited\n' > "$FAKE_DOCKER_STATE"
    ;;
  rm)
    command rm -f "$FAKE_DOCKER_STATE"
    ;;
  volume)
    case "$2" in
      inspect) if [ ! -f "$FAKE_DOCKER_VOLUME_STATE" ]; then exit 1; fi ;;
      create) printf 'created\n' > "$FAKE_DOCKER_VOLUME_STATE" ;;
    esac
    ;;
  cp)
    if [ -n "$FAKE_DOCKER_TAR" ]; then command cat "$FAKE_DOCKER_TAR"; fi
    ;;
  exec)
    case "$5" in
      'test -f '* ) exit 1 ;;
    esac
    ;;
esac
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	t.Setenv("FAKE_DOCKER_STATE", statePath)
	t.Setenv("FAKE_DOCKER_VOLUME_STATE", volumeStatePath)
	return logPath, statePath
}

func TestManagerCreatesPrivateTaskVolumeAtPublication(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	// The test provider normally skips Docker allocation. Use a product-like
	// provider only for this Manager publication boundary; no model is called.
	store.mu.Lock()
	store.config.Provider = "openai-compatible"
	store.mu.Unlock()
	requestedWorkspace := t.TempDir()
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	task, issue, err := manager.CreateTask(CreateIssueInput{
		Title: "private volume task", Objective: "write " + requestedWorkspace + "/report.md",
		Priority: "medium", WorkMode: "autonomous", Workspace: requestedWorkspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Workspace != TaskWorkspacePath || issue.Workspace != TaskWorkspacePath || strings.Contains(issue.Objective, requestedWorkspace) || !strings.Contains(issue.Objective, TaskWorkspacePath+"/report.md") {
		t.Fatalf("task=%+v issue=%+v", task, issue)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	wantVolume := taskContainerVolumeName(task.ContainerID)
	if !strings.Contains(logText, "volume create") || !strings.Contains(logText, wantVolume) {
		t.Fatalf("Task publication did not allocate named volume %q:\n%s", wantVolume, logText)
	}
	if strings.Contains(logText, "run --detach") {
		t.Fatalf("Task publication should allocate the workspace without starting the container:\n%s", logText)
	}
}

func TestWorkspaceIsolationMigrationRewritesLegacyHostPaths(t *testing.T) {
	installFakeDocker(t)
	store := configuredStore(t)
	task, issue, err := store.CreateTask(CreateIssueInput{Title: "legacy task", Objective: "initial", Priority: "medium", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	legacy := "/Users/example/aegis_workspace_3"
	if err = store.db.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{"workspace": legacy, "objective": "clone into " + legacy + "/repo"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"workspace": legacy, "description": "inspect " + legacy + "/repo"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&ContainerProfile{}).Where("id = ?", task.ContainerProfileID).Update("workspace_path", "/work").Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&ContainerInstance{}).Where("id = ?", task.ContainerID).Update("workspace_path", "/work").Error; err != nil {
		t.Fatal(err)
	}
	if err = migrateTaskWorkspaceIsolation(store.db); err != nil {
		t.Fatal(err)
	}
	task, _ = store.GetTask(task.ID)
	issue, _ = store.GetIssue(issue.ID)
	profile, _ := store.GetContainerProfile(task.ContainerProfileID)
	container, _ := store.GetContainer(task.ContainerID)
	if task.Workspace != TaskWorkspacePath || issue.Workspace != TaskWorkspacePath || profile.WorkspacePath != TaskWorkspacePath || container.WorkspacePath != TaskWorkspacePath {
		t.Fatalf("migration did not normalize workspaces: task=%+v issue=%+v profile=%+v container=%+v", task, issue, profile, container)
	}
	if task.Objective != "clone into /workspace/repo" || issue.Description != "inspect /workspace/repo" {
		t.Fatalf("migration retained legacy host paths: task=%q issue=%q", task.Objective, issue.Description)
	}
}

func TestTaskWorkspaceDoesNotCopyOrBrowseContainerContents(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{Name: "private workspace", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := store.CreateTask(CreateIssueInput{Title: "private task", Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.TaskWorkspace(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Root != TaskWorkspacePath || len(workspace.Entries) != 0 {
		t.Fatalf("container workspace contents were exposed: %+v", workspace)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "cp ") {
		t.Fatalf("workspace endpoint copied container files:\n%s", data)
	}
}

func TestSavingContainerProfileOnlyPersistsConfiguration(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "on-demand worker", WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID == "" || len(store.Containers()) != 0 {
		t.Fatalf("saving a profile unexpectedly created a container: profile=%+v containers=%+v", profile, store.Containers())
	}
	if data, readErr := os.ReadFile(logPath); readErr == nil && strings.TrimSpace(string(data)) != "" {
		t.Fatalf("saving a profile called Docker:\n%s", data)
	}
}

func TestTaskCreationBindsPersistentContainerAndExecutionStartsIt(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "persistent worker", WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, issue, err := store.CreateTask(CreateIssueInput{
		Title: "run in an on-demand container", Objective: "the task is queued",
		Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
		Workspace: t.TempDir(), ContainerProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ContainerID == "" || issue.ContainerID != task.ContainerID {
		t.Fatalf("task creation did not bind one container: task=%+v issue=%+v", task, issue)
	}
	containers := store.Containers()
	if len(containers) != 1 || containers[0].TaskID != task.ID || containers[0].RuntimeStatus != "missing" {
		t.Fatalf("unexpected container allocation before execution: %+v", containers)
	}
	container, err := store.ensureTaskContainer(issue)
	if err != nil {
		t.Fatal(err)
	}
	if container.RuntimeStatus != "running" || container.TaskID != task.ID || container.ContainerProfileID != profile.ID {
		t.Fatalf("unexpected task container: %+v", container)
	}
	task, err = store.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	issue, err = store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ContainerID != container.ID || issue.ContainerID != container.ID {
		t.Fatalf("container binding was not persisted: task=%q issue=%q container=%q", task.ContainerID, issue.ContainerID, container.ID)
	}
	now := time.Now()
	child := Issue{
		ID: nextID("issue"), Number: now.UnixNano(), Identifier: "TEST-CONTAINER-CHILD", ParentID: issue.ID,
		Title: "child container work", Objective: "reuse the task container", Status: "todo", ExecutionPhase: "active",
		Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID, ContainerID: container.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err = store.db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	childContainer, err := store.ensureTaskContainer(child)
	if err != nil {
		t.Fatalf("child Issue could not resolve its root task container: %v", err)
	}
	if childContainer.ID != container.ID || childContainer.TaskID != task.ID {
		t.Fatalf("child Issue resolved a different task container: %+v", childContainer)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedVolume := taskContainerVolumeName(container.ID) + ":/workspace"
	if !strings.Contains(string(logData), "--volume "+expectedVolume) {
		t.Fatalf("docker run does not use task volume %q:\n%s", expectedVolume, logData)
	}
	if strings.Contains(string(logData), task.Workspace+":/workspace") {
		t.Fatalf("task host workspace was mounted into the container:\n%s", logData)
	}
	for _, forbidden := range []string{"/aegis/sessions", "/aegis/runtime", "/aegis/skills"} {
		if strings.Contains(string(logData), "--volume") && strings.Contains(string(logData), forbidden) {
			t.Fatalf("container retained host bind mount %q:\n%s", forbidden, logData)
		}
	}
	stopped, err := store.StopContainer(container.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.RuntimeStatus != "exited" {
		t.Fatalf("runtime status after stop = %q", stopped.RuntimeStatus)
	}
}

func TestUnsourcedRootIssueIsPromotedToTaskWithContainer(t *testing.T) {
	installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "root task worker", WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateIssue(CreateIssueInput{
		Title: "promote this root", Objective: "bind task and container",
		Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.TaskSourceID == "" || root.ContainerID == "" {
		t.Fatalf("root Issue is not fully bound: %+v", root)
	}
	task, err := store.GetTask(root.TaskSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ContainerID != root.ContainerID {
		t.Fatalf("task/root container mismatch: task=%q root=%q", task.ContainerID, root.ContainerID)
	}
}

func TestFinishedTaskAutomaticallyStopsContainerAndKeepsWorkspaceVolume(t *testing.T) {
	logPath, statePath := installFakeDocker(t)
	store := configuredStore(t)
	task, root, err := store.CreateTask(CreateIssueInput{Title: "auto stop task", Objective: "finish and release runtime", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	container, err := store.ensureTaskContainer(root)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "last child", Objective: "finish first", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", root.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	root, _ = store.GetIssue(root.ID)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.ReconcileIssue(root)
	time.Sleep(50 * time.Millisecond)
	if state, _ := os.ReadFile(statePath); strings.TrimSpace(string(state)) != "running" {
		t.Fatalf("container stopped while a child Issue was unfinished: %q", state)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	child, _ = store.GetIssue(child.ID)
	manager.ReconcileIssue(child)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, _ := os.ReadFile(statePath)
		if strings.TrimSpace(string(state)) == "exited" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, _ := os.ReadFile(statePath)
	if strings.TrimSpace(string(state)) != "exited" {
		t.Fatalf("finished task container was not stopped: %q", state)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "stop --time 5 "+container.Name) {
		t.Fatalf("docker stop was not called for task %s: %s", task.ID, logText)
	}
	if strings.Contains(logText, "volume rm") || strings.Contains(logText, "rm -f "+container.Name) {
		t.Fatalf("automatic stop deleted the task runtime or workspace volume: %s", logText)
	}
	var events []ExecutionEvent
	eventDeadline := time.Now().Add(time.Second)
	for time.Now().Before(eventDeadline) {
		events = nil
		if err = store.db.Where("issue_id = ? AND type = ?", root.ID, "container").Find(&events).Error; err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(events) != 1 || !strings.Contains(events[0].Title, "自动停止") {
		t.Fatalf("automatic container stop event missing: %+v", events)
	}
}

func TestFinishedTaskWaitsForActiveExecutionBeforeStoppingContainer(t *testing.T) {
	_, statePath := installFakeDocker(t)
	store := configuredStore(t)
	_, root, err := store.CreateTask(CreateIssueInput{Title: "active execution guard", Objective: "do not stop early", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ensureTaskContainer(root); err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(root, root.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.updateExecution(execution.ID, map[string]any{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", root.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	root, _ = store.GetIssue(root.ID)
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.ReconcileIssue(root)
	time.Sleep(50 * time.Millisecond)
	state, _ := os.ReadFile(statePath)
	if strings.TrimSpace(string(state)) != "running" {
		t.Fatalf("container stopped while an Execution was active: %q", state)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "completed", "finished_at": now}); err != nil {
		t.Fatal(err)
	}
	manager.ReconcileIssue(root)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, _ = os.ReadFile(statePath)
		if strings.TrimSpace(string(state)) == "exited" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("container remained running after the active Execution ended: %q", state)
}

func TestRootIssueCreationAutomaticallyCreatesTask(t *testing.T) {
	installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "standalone issue", WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateIssue(CreateIssueInput{
		Title: "standalone root", Objective: "run without a Task", Priority: "medium",
		WorkMode: "autonomous", ContainerProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{
		ParentID: root.ID, Title: "standalone child", Objective: "reuse the Issue tree container",
		Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	container, err := store.ensureTaskContainer(child)
	if err != nil {
		t.Fatalf("standalone Issue required a Task definition: %v", err)
	}
	if root.TaskSourceID == "" || container.TaskID != root.TaskSourceID {
		t.Fatalf("root Issue was not promoted to a container-backed Task: root=%+v container=%+v", root, container)
	}
	root, _ = store.GetIssue(root.ID)
	child, _ = store.GetIssue(child.ID)
	if root.ContainerID != container.ID || child.ContainerID != container.ID {
		t.Fatalf("standalone container binding missing: root=%q child=%q container=%q", root.ContainerID, child.ContainerID, container.ID)
	}
}

func TestDeleteContainerRequiresConfirmationAndCascadesTaskData(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "isolated worker", WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, root, err := store.CreateTask(CreateIssueInput{
		Title: "container task", Objective: "verify cascade", Priority: "medium",
		WorkMode: "autonomous", Workspace: t.TempDir(), ContainerProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	container, err := store.ensureTaskContainer(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	child := Issue{
		ID: nextID("issue"), Number: now.UnixNano(), Identifier: "TEST-DELETE-CHILD", ParentID: root.ID,
		Title: "child", Objective: "child work", Status: "in_progress", ExecutionPhase: "active",
		Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID, ContainerID: container.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err = store.db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	execution := Execution{
		ID: nextID("execution"), IssueID: child.ID, AgentID: "backend-engineer", Kind: "work",
		Status: "running", SessionID: nextID("pi-session"), StartedAt: now, UpdatedAt: now,
	}
	if err = store.db.Create(&execution).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&ExecutionEvent{ID: nextID("event"), ExecutionID: execution.ID, IssueID: child.ID, Type: "tool", Title: "running", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&IssueComment{ID: nextID("comment"), IssueID: child.ID, ExecutionID: execution.ID, Type: "normal", Body: "progress", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	impact, err := store.ContainerDeleteImpact(container.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.TaskID != task.ID || impact.IssueCount != 2 || impact.ExecutionCount != 1 || impact.ActiveExecutionCount != 1 {
		t.Fatalf("unexpected delete impact: %+v", impact)
	}
	if _, err = store.DeleteContainer(container.ID, false); !errors.Is(err, ErrContainerProfileReferenced) {
		t.Fatalf("delete without confirmation error = %v", err)
	}
	if _, err = store.DeleteContainerProfile(profile.ID, true); !errors.Is(err, ErrContainerProfileReferenced) {
		t.Fatalf("profile deletion should be blocked while a container exists: %v", err)
	}

	result, err := store.DeleteContainer(container.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedIssues != 2 || result.DeletedTasks != 1 || result.DeletedExecutions != 1 {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	for name, model := range map[string]any{
		"container": &ContainerInstance{}, "task": &Task{}, "issue": &Issue{},
		"execution": &Execution{}, "event": &ExecutionEvent{}, "comment": &IssueComment{},
	} {
		var count int64
		query := store.db.Model(model)
		switch name {
		case "container":
			query = query.Where("id = ?", container.ID)
		case "task":
			query = query.Where("id = ?", task.ID)
		case "issue":
			query = query.Where("id IN ?", []string{root.ID, child.ID})
		case "execution":
			query = query.Where("id = ?", execution.ID)
		case "event":
			query = query.Where("execution_id = ?", execution.ID)
		case "comment":
			query = query.Where("issue_id = ?", child.ID)
		}
		if err = query.Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s remains after cascade: count=%d err=%v", name, count, err)
		}
	}
	if _, err = store.GetContainerProfile(profile.ID); err != nil {
		t.Fatalf("deleting a task container also deleted its environment profile: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "volume rm -f "+taskContainerVolumeName(container.ID)) {
		t.Fatalf("container volume was not deleted:\n%s", logData)
	}
}

func TestBatchStopImpactAndDeleteContainers(t *testing.T) {
	logPath, _ := installFakeDocker(t)
	store := configuredStore(t)
	containers := make([]ContainerInstance, 0, 2)
	for _, title := range []string{"batch one", "batch two"} {
		_, root, err := store.CreateTask(CreateIssueInput{
			Title: title, Objective: "batch lifecycle", Priority: "medium",
			WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
		})
		if err != nil {
			t.Fatal(err)
		}
		container, err := store.ensureTaskContainer(root)
		if err != nil {
			t.Fatal(err)
		}
		containers = append(containers, container)
	}

	stopped, err := store.StopContainers([]string{containers[0].ID, containers[1].ID, containers[0].ID, "missing-container"})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Requested != 3 || len(stopped.Stopped) != 2 || len(stopped.Failed) != 1 || stopped.Failed[0].ContainerID != "missing-container" {
		t.Fatalf("unexpected batch stop result: %+v", stopped)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "stop --time 5 "+containers[0].Name) {
		t.Fatalf("running container was not stopped:\n%s", logData)
	}

	impact, err := store.ContainerBatchDeleteImpact([]string{containers[0].ID, containers[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if impact.ContainerCount != 2 || impact.TaskCount != 2 || impact.IssueCount != 2 || impact.ExecutionCount != 0 || len(impact.Items) != 2 {
		t.Fatalf("unexpected batch impact: %+v", impact)
	}

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	deleted, err := manager.DeleteContainers([]string{containers[0].ID, containers[1].ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Requested != 2 || len(deleted.Deleted) != 2 || len(deleted.Failed) != 0 || deleted.DeletedTasks != 2 || deleted.DeletedIssues != 2 {
		t.Fatalf("unexpected batch delete result: %+v", deleted)
	}
	var remaining int64
	if err = store.db.Model(&ContainerInstance{}).Where("id IN ?", []string{containers[0].ID, containers[1].ID}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("batch delete left %d containers", remaining)
	}
}
