package control

import (
	"context"
	"encoding/json"
	"errors"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

// NativeConciergeSource lets the in-process concierge create a real Task.
// It deliberately exposes no legacy RPC transport or execution control token.
type NativeConciergeSource struct{ Manager *Manager }

func (s NativeConciergeSource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Manager == nil || s.Manager.store == nil {
		return capability.Resolved{}, errors.New("native concierge: manager is required")
	}
	var durable Execution
	if err := s.Manager.store.db.First(&durable, "id = ? AND agent_id = ? AND kind = ?", execution.ExecutionID, conciergeAgentID, "concierge").Error; err != nil {
		return capability.Resolved{}, errors.New("native concierge: concierge Execution is required")
	}
	tool := agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_create_task", Description: "Create one real top-level Aegis Task and hand it to the Go AgentCore Coordination runtime. Every operator attachment in this concierge conversation is copied into the new Task; the conversation source files remain available for later Task creations and retries.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","minLength":1},"taskDescription":{"type":"string"},"objective":{"type":"string"},"priority":{"type":"string","enum":["low","middle","high"]},"workMode":{"type":"string","enum":["autonomous","guided"]},"agentId":{"type":"string","minLength":1},"workspace":{"type":"string"},"constraints":{"type":"string"}},"required":["title","agentId"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input CreateConciergeTaskInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		issue, err := s.Manager.createTaskFromConcierge(durable.ID, input)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		encoded, _ := json.Marshal(map[string]any{"issueId": issue.ID, "identifier": issue.Identifier, "title": issue.Title, "created": true})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
	return capability.Resolved{
		Tools:    []agentcore.Tool{tool},
		Snapshot: capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: ref.Version, Metadata: map[string]string{"runtime": "agentcore"}},
	}, nil
}

var _ capability.Source = NativeConciergeSource{}
