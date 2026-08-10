package agenthost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/z3r2ne/agentcore"
)

func TestInvocationMetadataDecoratorAddsSchemaAndStripsExecutionArgument(t *testing.T) {
	var received map[string]any
	base := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name:       "publish",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"description":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		},
		Mode:   agentcore.ToolExecutionSequential,
		Policy: agentcore.ToolPolicy{Timeout: 3 * time.Second},
		ExecuteFunc: func(_ context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			if err := json.Unmarshal(raw, &received); err != nil {
				return agentcore.ToolResult{}, err
			}
			return agentcore.TextToolResult("ok"), nil
		},
	}
	decorated, err := decorateInvocationMetadata([]agentcore.Tool{base})
	if err != nil {
		t.Fatal(err)
	}
	tool := decorated[0]
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(tool.Definition().Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties[invocationDescriptionParameter] == nil || schema.Properties["description"] == nil {
		t.Fatalf("decorated schema lost invocation or business description: %s", tool.Definition().Parameters)
	}
	if containsString(schema.Required, invocationDescriptionParameter) {
		t.Fatalf("invocation description must remain backward-compatible: %v", schema.Required)
	}
	if mode := tool.(agentcore.ToolExecutionModeProvider).ExecutionMode(); mode != agentcore.ToolExecutionSequential {
		t.Fatalf("execution mode=%q", mode)
	}
	if policy := tool.(agentcore.ToolPolicyProvider).ToolPolicy(); policy.Timeout != 3*time.Second {
		t.Fatalf("tool policy=%+v", policy)
	}
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"path":"report.md","description":"artifact summary","invocationDescription":"publish final evidence"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := received[invocationDescriptionParameter]; exists {
		t.Fatalf("metadata leaked into base tool arguments: %+v", received)
	}
	if received["description"] != "artifact summary" {
		t.Fatalf("business description was stripped: %+v", received)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
