package control

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDockerPiCommandBuildsIsolatedRuntime(t *testing.T) {
	dataDir := t.TempDir()
	profile := ContainerProfile{
		Image: "example/pi:latest", NodePath: "node", PiPath: "/opt/pi/cli.js",
		WorkspacePath: "/workspace", NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1.5,
	}
	command, args := dockerPiCommand(profile, "aegis-execution-1", "/host/project", dataDir, "/host/guard.ts", Config{Provider: "openai"}, []string{"--mode", "rpc"}, []string{
		"PATH=/host/bin", "HOME=/secret/home", "OPENAI_API_KEY=test-key",
		"AEGIS_CONTROL_URL=http://127.0.0.1:8080", "AEGIS_WORKSPACE=/workspace",
	}, "http://127.0.0.1:8080")
	if command != "docker" {
		t.Fatalf("command = %q", command)
	}
	joined := strings.Join(args, "\n")
	for _, expected := range []string{
		"aegis-execution-1", "/host/project:/workspace",
		filepath.Join(dataDir, "sessions") + ":/aegis/sessions",
		"AEGIS_CONTROL_URL=http://host.docker.internal:8080",
		"OPENAI_API_KEY=test-key", "example/pi:latest", "/opt/pi/cli.js",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("docker arguments do not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"PATH=/host/bin", "HOME=/secret/home"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("host environment leaked into container: %q", forbidden)
		}
	}
	if !slices.Contains(args, "--rm") {
		t.Error("container must be ephemeral")
	}
}

func TestContainerHostURLOnlyRewritesLoopbackHost(t *testing.T) {
	if got := containerHostURL("http://localhost:3000/api"); got != "http://host.docker.internal:3000/api" {
		t.Fatalf("localhost rewrite = %q", got)
	}
	if got := containerHostURL("https://api.example.test/v1"); got != "https://api.example.test/v1" {
		t.Fatalf("external URL changed = %q", got)
	}
}

func TestDeleteContainerProfileRequiresConfirmationAndCascadesIssueData(t *testing.T) {
	store := configuredStore(t)
	profile, err := store.SaveContainerProfile("", SaveContainerProfileInput{
		Name: "isolated worker", HostWorkspace: t.TempDir(), WorkspacePath: "/workspace",
		NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	task := Task{
		ID: nextID("task"), Title: "container task", Objective: "verify cascade", Priority: "medium",
		WorkMode: "autonomous", ContainerProfileID: profile.ID, CreatedAt: now, UpdatedAt: now,
	}
	root := Issue{
		ID: nextID("issue"), Number: now.UnixNano(), Identifier: "TEST-DELETE-ROOT", TaskSourceID: task.ID,
		Title: task.Title, Objective: task.Objective, Status: "in_progress", ExecutionPhase: "active",
		Priority: task.Priority, WorkMode: task.WorkMode, ContainerProfileID: profile.ID, CreatedAt: now, UpdatedAt: now,
	}
	child := Issue{
		ID: nextID("issue"), Number: now.UnixNano() + 1, Identifier: "TEST-DELETE-CHILD", ParentID: root.ID,
		Title: "child", Objective: "child work", Status: "in_progress", ExecutionPhase: "active",
		Priority: "medium", WorkMode: "autonomous", ContainerProfileID: profile.ID, CreatedAt: now, UpdatedAt: now,
	}
	if err = store.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	// Descendants belong to the deleted tree even if a future edit gives them a
	// different runtime reference.
	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Update("container_profile_id", "").Error; err != nil {
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

	impact, err := store.ContainerProfileDeleteImpact(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.IssueCount != 2 || impact.TaskCount != 1 || impact.ExecutionCount != 1 || impact.ActiveExecutionCount != 1 {
		t.Fatalf("unexpected delete impact: %+v", impact)
	}
	if _, err = store.DeleteContainerProfile(profile.ID, false); !errors.Is(err, ErrContainerProfileReferenced) {
		t.Fatalf("delete without confirmation error = %v", err)
	}

	result, err := store.DeleteContainerProfile(profile.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedIssues != 2 || result.DeletedTasks != 1 || result.DeletedExecutions != 1 {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	for name, model := range map[string]any{
		"profile": &ContainerProfile{}, "task": &Task{}, "issue": &Issue{},
		"execution": &Execution{}, "event": &ExecutionEvent{}, "comment": &IssueComment{},
	} {
		var count int64
		query := store.db.Model(model)
		switch name {
		case "profile":
			query = query.Where("id = ?", profile.ID)
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
}
