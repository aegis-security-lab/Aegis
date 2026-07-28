package control

import (
	"archive/tar"
	"bytes"
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
    ;;
  cp)
    if [ -n "$FAKE_DOCKER_TAR" ]; then command cat "$FAKE_DOCKER_TAR"; fi
    ;;
esac
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	t.Setenv("FAKE_DOCKER_STATE", statePath)
	return logPath, statePath
}

func TestContainerWorkspaceIsListedDirectlyFromDockerArchive(t *testing.T) {
	_, statePath := installFakeDocker(t)
	if err := os.WriteFile(statePath, []byte("exited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	now := time.Now().Truncate(time.Second)
	for _, header := range []*tar.Header{
		{Name: "./nested/", Typeflag: tar.TypeDir, Mode: 0o700, ModTime: now},
		{Name: "./nested/report.txt", Typeflag: tar.TypeReg, Mode: 0o600, Size: 6, ModTime: now},
	} {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := writer.Write([]byte("report")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "workspace.tar")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DOCKER_TAR", archivePath)
	workspace, err := containerTaskWorkspace(ContainerInstance{
		Name: "aegis-task-workspace", WorkspacePath: "/workspace", RuntimeStatus: "exited",
	})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Root != "/workspace" || len(workspace.Entries) != 2 {
		t.Fatalf("unexpected container workspace: %+v", workspace)
	}
	if workspace.Entries[0].Path != "nested" || workspace.Entries[0].Kind != "directory" || workspace.Entries[1].Path != "nested/report.txt" || workspace.Entries[1].Size != 6 {
		t.Fatalf("unexpected container workspace entries: %+v", workspace.Entries)
	}
}

func TestContainerWorkspaceHidesAegisRuntimeFiles(t *testing.T) {
	_, statePath := installFakeDocker(t)
	if err := os.WriteFile(statePath, []byte("running\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, header := range []*tar.Header{
		{Name: "./.aegis/", Typeflag: tar.TypeDir, Mode: 0o700},
		{Name: "./.aegis/issues/issue-1/", Typeflag: tar.TypeDir, Mode: 0o700},
		{Name: "./report.md", Typeflag: tar.TypeReg, Mode: 0o600, Size: 6},
	} {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			_, _ = writer.Write([]byte("report"))
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "workspace.tar")
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DOCKER_TAR", archivePath)
	workspace, err := containerTaskWorkspace(ContainerInstance{
		Name: "aegis-task-clean", WorkspacePath: "/workspace", RuntimeStatus: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Entries) != 1 || workspace.Entries[0].Path != "report.md" {
		t.Fatalf("runtime files leaked into task workspace: %+v", workspace.Entries)
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
