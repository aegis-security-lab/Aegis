package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

// NativeValidationSource exposes only the state-changing acceptance decisions.
// The validator receives ordinary workspace tools separately and reads the
// Worker's materialized submission evidence from paths in its prompt.
type NativeValidationSource struct{ Manager *Manager }

func (s NativeValidationSource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Manager == nil || s.Manager.store == nil {
		return capability.Resolved{}, errors.New("native validation: manager is required")
	}
	var durable Execution
	if err := s.Manager.store.db.First(&durable, "id = ? AND kind = ?", execution.ExecutionID, "validation").Error; err != nil {
		return capability.Resolved{}, errors.New("native validation: active validation Execution is required")
	}
	if _, err := s.Manager.activeValidationForExecution(durable.ID); err != nil {
		return capability.Resolved{}, err
	}
	binding := nativeValidationBinding{manager: s.Manager, executionID: durable.ID}
	return capability.Resolved{
		Tools: []agentcore.Tool{
			binding.submitDecisionTool(), binding.closeIssueTool(),
		},
		Instructions: []capability.Instruction{{
			Source:  "aegis-validation",
			Content: "Inspect the Worker submission and its local attachment paths with the ordinary workspace tools, then call exactly one structured validation decision tool. Do not modify the source evidence or Worker deliverables. Network, delegation, delivery, and Phone access are unavailable.",
		}},
		Snapshot: capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: ref.Version, Metadata: map[string]string{"scope": "validation-decisions"}},
	}, nil
}

type nativeValidationBinding struct {
	manager     *Manager
	executionID string
}

func (b nativeValidationBinding) submitDecisionTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_submit_validation", Description: "Submit a retry or abandoned acceptance decision and end this validation turn.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"outcome":{"type":"string","enum":["retry","abandoned"]},"summary":{"type":"string","minLength":1},"feedback":{"type":"string"},"impossibilityProof":{"type":"string"}},"required":["outcome","summary"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input SubmitValidationDecisionInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if strings.TrimSpace(input.Outcome) == "passed" {
			return agentcore.ToolResult{}, errors.New("use aegis_close_current_issue for a passing decision")
		}
		decision, err := b.manager.submitValidationDecision(b.executionID, input)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		encoded, _ := json.Marshal(decision)
		result := agentcore.TextToolResult(string(encoded))
		result.Terminate = true
		return result, nil
	}}
}

func (b nativeValidationBinding) closeIssueTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_close_current_issue", Description: "Submit a passing acceptance decision after verifying the objective and published evidence, then end this validation turn.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string","minLength":1},"evidenceCommentId":{"type":"string"}},"required":["summary"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input CloseValidatedIssueInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		decision, err := b.manager.closeValidatedIssue(b.executionID, input)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		encoded, _ := json.Marshal(decision)
		result := agentcore.TextToolResult(string(encoded))
		result.Terminate = true
		return result, nil
	}}
}

var _ capability.Source = NativeValidationSource{}
