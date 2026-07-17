package control

import (
	"encoding/json"
	"testing"
)

func TestToolEventLifecycleUsesSingleStructuredRecord(t *testing.T) {
	store := configuredStore(t)
	store.startToolEvent("exec-1", "issue-1", "call-1", "write", map[string]any{
		"path": "internal/app.go", "content": "package app\n\nfunc Run() {}\n",
	})
	store.finishToolEvent("exec-1", "issue-1", "call-1", "write", map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "Wrote 3 lines"}},
	}, false)

	var events []ExecutionEvent
	if err := store.db.Where("execution_id = ?", "exec-1").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one correlated tool event, got %d", len(events))
	}
	event := events[0]
	if event.ToolCallID != "call-1" || event.ToolName != "write" || event.Status != "completed" || event.IsError {
		t.Fatalf("unexpected event metadata: %+v", event)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(event.InputJSON), &input); err != nil {
		t.Fatalf("input is not JSON: %v", err)
	}
	if input["path"] != "internal/app.go" {
		t.Fatalf("input path=%v", input["path"])
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(event.OutputJSON), &output); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if output["content"] == nil {
		t.Fatalf("missing structured output: %+v", output)
	}
}

func TestToolEventEndWithoutStartStillPersists(t *testing.T) {
	store := configuredStore(t)
	store.finishToolEvent("exec-2", "issue-2", "call-orphan", "bash", map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "failed"}},
	}, true)
	var event ExecutionEvent
	if err := store.db.Where("execution_id = ?", "exec-2").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.Status != "failed" || !event.IsError || event.ToolName != "bash" {
		t.Fatalf("unexpected orphan completion: %+v", event)
	}
}
