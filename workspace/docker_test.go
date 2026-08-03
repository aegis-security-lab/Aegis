package workspace

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type schemaModel struct{}

func (schemaModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	return schemaStream{}, nil
}

type schemaStream struct{}

func (schemaStream) Recv() (agentcore.ModelChunk, error) { return agentcore.ModelChunk{}, io.EOF }
func (schemaStream) Close() error                        { return nil }

func TestDockerSourceToolsHaveValidSchemasAndContainerOnlyRuntime(t *testing.T) {
	binary, logPath, inputPath := fakeDocker(t)
	source := DockerSource{Binary: binary, MaxOutputBytes: 1024}
	resolved, err := source.Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: "execution-1", Workspace: "/workspace",
		Values: map[string]any{"control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools) != 7 {
		t.Fatalf("tools=%d", len(resolved.Tools))
	}
	if _, err = agentcore.New(agentcore.Config{Model: schemaModel{}, Tools: resolved.Tools}); err != nil {
		t.Fatalf("workspace tool schema is not accepted by AgentCore: %v", err)
	}

	bash := namedTool(t, resolved.Tools, "bash")
	result, err := bash.Execute(context.Background(), json.RawMessage(`{"command":"pwd"}`), nil)
	if err != nil || result.IsError || !strings.Contains(result.Text(), "container-output") {
		t.Fatalf("bash result=%+v err=%v", result, err)
	}
	write := namedTool(t, resolved.Tools, "write")
	result, err = write.Execute(context.Background(), json.RawMessage(`{"path":"reports/result.md","content":"container data"}`), nil)
	if err != nil || result.IsError || !strings.Contains(result.Text(), "/workspace/reports/result.md") {
		t.Fatalf("write result=%+v err=%v", result, err)
	}
	content, err := os.ReadFile(inputPath)
	if err != nil || string(content) != "container data" {
		t.Fatalf("docker stdin=%q err=%v", content, err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "exec -i --workdir /workspace task-container sh -lc pwd") || !strings.Contains(logText, "/workspace/reports/result.md") {
		t.Fatalf("tools did not target the Task container and workspace:\n%s", logText)
	}
}

func TestDockerSourceRejectsMissingContainerAndEscapingPaths(t *testing.T) {
	if _, err := (DockerSource{}).Resolve(context.Background(), capability.ResolveContext{Workspace: "/workspace"}, capability.Ref{Kind: capability.KindTool, Name: "workspace"}); err == nil {
		t.Fatal("expected missing Task container to be rejected")
	}
	binary, logPath, _ := fakeDocker(t)
	resolved, err := (DockerSource{Binary: binary}).Resolve(context.Background(), capability.ResolveContext{
		Workspace: "/workspace", Values: map[string]any{"control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	read := namedTool(t, resolved.Tools, "read")
	if _, err = read.Execute(context.Background(), json.RawMessage(`{"path":"../../etc/passwd"}`), nil); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaping path error=%v", err)
	}
	if data, readErr := os.ReadFile(logPath); readErr == nil && len(data) != 0 {
		t.Fatalf("invalid path reached Docker:\n%s", data)
	}
}

func TestDockerSourceCanMaterializeReadOnlyDiscoveryTools(t *testing.T) {
	binary, _, _ := fakeDocker(t)
	resolved, err := (DockerSource{Binary: binary}).Resolve(context.Background(), capability.ResolveContext{
		Workspace: "/workspace", Values: map[string]any{"control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "workspace", Config: map[string]any{
		"allowShell": false,
		"allowWrite": false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Tools) != 4 {
		t.Fatalf("read-only tools=%d, want 4", len(resolved.Tools))
	}
	for _, name := range []string{"read", "grep", "find", "ls"} {
		_ = namedTool(t, resolved.Tools, name)
	}
	for _, forbidden := range []string{"bash", "write", "edit"} {
		for _, tool := range resolved.Tools {
			if tool.Definition().Name == forbidden {
				t.Fatalf("read-only workspace unexpectedly exposed %s", forbidden)
			}
		}
	}
	if resolved.Snapshot.Metadata["allowShell"] != "false" || resolved.Snapshot.Metadata["allowWrite"] != "false" {
		t.Fatalf("read-only capability metadata=%v", resolved.Snapshot.Metadata)
	}
}

func TestDockerToolHonorsContextCancellation(t *testing.T) {
	binary, _, _ := fakeDocker(t)
	resolved, err := (DockerSource{Binary: binary}).Resolve(context.Background(), capability.ResolveContext{
		Workspace: "/workspace", Values: map[string]any{"control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = namedTool(t, resolved.Tools, "bash").Execute(ctx, json.RawMessage(`{"command":"pwd"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation error=%v", err)
	}
}

func fakeDocker(t *testing.T) (binary, logPath, inputPath string) {
	t.Helper()
	dir := t.TempDir()
	binary = filepath.Join(dir, "docker")
	logPath = filepath.Join(dir, "docker.log")
	inputPath = filepath.Join(dir, "stdin")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$WORKSPACE_DOCKER_LOG"
case "$*" in
  *'cat > "$1"'*) command cat > "$WORKSPACE_DOCKER_STDIN" ;;
  *) printf 'container-output\n' ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKSPACE_DOCKER_LOG", logPath)
	t.Setenv("WORKSPACE_DOCKER_STDIN", inputPath)
	return binary, logPath, inputPath
}

func namedTool(t *testing.T, tools []agentcore.Tool, name string) agentcore.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}
