package control

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

type nativeDeliverySchemaModel struct{}

func (nativeDeliverySchemaModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	return nativeDeliverySchemaStream{}, nil
}

type nativeDeliverySchemaStream struct{}

func (nativeDeliverySchemaStream) Recv() (agentcore.ModelChunk, error) {
	return agentcore.ModelChunk{}, io.EOF
}
func (nativeDeliverySchemaStream) Close() error { return nil }

func TestNativeDeliveryPublishesContainerFileAndSubmitsResult(t *testing.T) {
	content := "verified container report"
	fakeBin := t.TempDir()
	dockerPath := filepath.Join(fakeBin, "docker")
	logPath := filepath.Join(t.TempDir(), "docker.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$NATIVE_DELIVERY_DOCKER_LOG"
if [ "$3" = "sh" ]; then
  printf '%s\n' "$NATIVE_DELIVERY_SIZE"
elif [ "$3" = "cat" ]; then
  printf '%s' "$NATIVE_DELIVERY_CONTENT"
fi
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NATIVE_DELIVERY_DOCKER_LOG", logPath)
	t.Setenv("NATIVE_DELIVERY_CONTENT", content)
	t.Setenv("NATIVE_DELIVERY_SIZE", "25")

	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Native delivery", Objective: "Publish evidence", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (NativeDeliverySource{Store: store}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: execution.ID, AgentID: issue.AssigneeAgentID, Workspace: TaskWorkspacePath,
		Values: map[string]any{controlIssueIDValue: issue.ID, "control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "delivery"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = agentcore.New(agentcore.Config{Model: nativeDeliverySchemaModel{}, Tools: resolved.Tools}); err != nil {
		t.Fatalf("native delivery tool schema is invalid: %v", err)
	}
	publish := controlNamedTool(t, resolved.Tools, "aegis_publish_attachment")
	result, err := publish.Execute(context.Background(), json.RawMessage(`{"path":"reports/audit.md","description":"audit evidence"}`), nil)
	if err != nil || result.IsError || !strings.Contains(result.Text(), `"published":true`) {
		t.Fatalf("publish result=%+v err=%v", result, err)
	}
	var attachments []IssueAttachment
	if err = store.db.Where("execution_id = ?", execution.ID).Find(&attachments).Error; err != nil || len(attachments) != 1 {
		t.Fatalf("attachments=%+v err=%v", attachments, err)
	}
	_, file, err := store.AttachmentFile(attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, readErr := io.ReadAll(file)
	_ = file.Close()
	if readErr != nil || string(stored) != content || attachments[0].SourcePath != "/workspace/reports/audit.md" {
		t.Fatalf("stored=%q attachment=%+v err=%v", stored, attachments[0], readErr)
	}
	if data, readErr := os.ReadFile(logPath); readErr != nil || !strings.Contains(string(data), "exec task-container cat -- /workspace/reports/audit.md") {
		t.Fatalf("Docker stream log=%q err=%v", data, readErr)
	}

	progress := controlNamedTool(t, resolved.Tools, "aegis_report_progress")
	if _, err = progress.Execute(context.Background(), json.RawMessage(`{"stage":"verify","summary":"report published","currentActivity":"submitting"}`), nil); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = store.db.Model(&ExecutionProgress{}).Where("execution_id = ?", execution.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("progress count=%d err=%v", count, err)
	}

	submit := controlNamedTool(t, resolved.Tools, "aegis_submit_final_result")
	result, err = submit.Execute(context.Background(), json.RawMessage(`{"body":"All evidence was published."}`), nil)
	if err != nil || !result.Terminate {
		t.Fatalf("submit result=%+v err=%v", result, err)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil || !execution.FinalResultSubmitted || execution.FinalResult != "All evidence was published." {
		t.Fatalf("execution=%+v err=%v", execution, err)
	}
}

func TestNativeDeliveryRejectsAttachmentOutsideTaskWorkspace(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Isolated", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := store.createExecution(issue, issue.AssigneeAgentID, "work")
	resolved, err := (NativeDeliverySource{Store: store}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: execution.ID, Workspace: TaskWorkspacePath,
		Values: map[string]any{controlIssueIDValue: issue.ID, "control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "delivery"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = controlNamedTool(t, resolved.Tools, "aegis_publish_attachment").Execute(context.Background(), json.RawMessage(`{"path":"/etc/passwd"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "不能离开") {
		t.Fatalf("outside path error=%v", err)
	}
}

func TestNativeDeliveryBudgetSummaryIsPhaseGatedAndTerminates(t *testing.T) {
	store := configuredStore(t)
	root, _ := store.CreateIssue(CreateIssueInput{Title: "Root", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	child, _ := store.CreateIssue(CreateIssueInput{ParentID: root.ID, Title: "Budgeted child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	execution, err := store.createExecution(child, child.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (NativeDeliverySource{Store: store}).Resolve(context.Background(), capability.ResolveContext{
		ExecutionID: execution.ID, AgentID: child.AssigneeAgentID, Workspace: TaskWorkspacePath,
		Values: map[string]any{controlIssueIDValue: child.ID, "control.runtimeId": "task-container"},
	}, capability.Ref{Kind: capability.KindTool, Name: "delivery"})
	if err != nil {
		t.Fatal(err)
	}
	submit := controlNamedTool(t, resolved.Tools, "aegis_submit_budget_summary")
	if _, err = submit.Execute(context.Background(), json.RawMessage(`{"body":"partial evidence"}`), nil); err == nil || !strings.Contains(err.Error(), "尚未进入") {
		t.Fatalf("active phase accepted budget summary: %v", err)
	}
	if err = store.db.Model(&Execution{}).Where("id = ?", execution.ID).Update("budget_phase", "summarizing").Error; err != nil {
		t.Fatal(err)
	}
	result, err := submit.Execute(context.Background(), json.RawMessage(`{"body":"completed A; B remains; continue this direction"}`), nil)
	if err != nil || !result.Terminate {
		t.Fatalf("summary result=%+v err=%v", result, err)
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil || execution.Result != "completed A; B remains; continue this direction" || execution.FinalResultSubmitted {
		t.Fatalf("execution=%+v err=%v", execution, err)
	}
}

func controlNamedTool(t *testing.T, tools []agentcore.Tool, name string) agentcore.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}
