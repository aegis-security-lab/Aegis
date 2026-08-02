package agentcoreadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

type Invocation struct {
	EventID     string
	IssueID     string
	ExecutionID string
	AgentID     string
}

type Client interface {
	Delegate(context.Context, Invocation, coordination.DelegationRequest) error
	Continue(context.Context, Invocation, coordination.ContinueRequest) error
	Wait(context.Context, Invocation, coordination.WaitRequest) error
}

type Source struct{ Client Client }

func (s Source) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Client == nil {
		return capability.Resolved{}, errors.New("coordination tools: client is required")
	}
	issueID, _ := execution.Values["control.issueId"].(string)
	base := Invocation{IssueID: strings.TrimSpace(issueID), ExecutionID: execution.ExecutionID, AgentID: execution.AgentID}
	if base.IssueID == "" {
		return capability.Resolved{}, errors.New("coordination tools: control.issueId is required")
	}
	delegate := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name:        "coordinate_delegate",
			Description: "Create and assign one or more child Board Issues. The parent Agent always continues working; every child may inherit, merge, or replace policy-approved capabilities.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"children": {
						"type": "array",
						"minItems": 1,
						"items": {
							"type": "object",
							"properties": {
								"agentId": {"type": "string", "minLength": 1},
								"title": {"type": "string"},
								"prompt": {"type": "string", "minLength": 1},
								"workspace": {"type": "string"},
								"capabilitySelection": {
									"type": "string",
									"enum": ["inherit", "merge", "replace"]
								},
								"capabilities": {
									"type": "array",
									"maxItems": 64,
									"items": {
										"type": "object",
										"properties": {
											"kind": {"type": "string", "minLength": 1},
											"name": {"type": "string", "minLength": 1},
											"version": {"type": "string"},
											"config": {"type": "object"},
											"optional": {"type": "boolean"}
										},
										"required": ["kind", "name"],
										"additionalProperties": false
									}
								}
							},
							"required": ["agentId", "prompt"],
							"additionalProperties": false
						}
					}
				},
				"required": ["children"],
				"additionalProperties": false
			}`),
		},
		Mode: agentcore.ToolExecutionSequential,
		ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			var request coordination.DelegationRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				return agentcore.ToolResult{}, err
			}
			invocation := withEventID(ctx, base, "delegate", raw)
			if err := s.Client.Delegate(ctx, invocation, request); err != nil {
				return agentcore.ToolResult{}, err
			}
			return agentcore.TextToolResult("Child Board Issues were created and assigned. Continue your own work; progress, messages, and completion will arrive asynchronously."), nil
		},
	}
	continueIssue := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{
			Name:        "coordinate_continue",
			Description: "Reopen one direct child Issue that ended as failed or budget_exceeded and dispatch a new Execution with a fresh per-Execution budget. This does not reset the Task wall-clock budget.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"childIssueId":{"type":"string","minLength":1},"reason":{"type":"string","minLength":1}},"required":["childIssueId","reason"],"additionalProperties":false}`),
		},
		Mode: agentcore.ToolExecutionSequential,
		ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			var request coordination.ContinueRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				return agentcore.ToolResult{}, err
			}
			invocation := withEventID(ctx, base, "continue", raw)
			if err := s.Client.Continue(ctx, invocation, request); err != nil {
				return agentcore.ToolResult{}, err
			}
			return agentcore.TextToolResult("The child Issue was reopened and a new Execution was dispatched with a fresh Execution budget. The Task wall-clock budget is unchanged."), nil
		},
	}
	sleep := agentcore.FuncTool{
		ToolDefinition: agentcore.ToolDefinition{Name: "coordinate_sleep", Description: "End useful work for now and request a durable wakeup after a delay. The system heartbeat has higher priority and may wake you earlier when task coordination needs attention.", Parameters: json.RawMessage(`{"type":"object","properties":{"childIds":{"type":"array","items":{"type":"string"}},"wakeAfterSeconds":{"type":"integer","minimum":1},"message":{"type":"string"}},"required":["wakeAfterSeconds"],"additionalProperties":false}`)},
		Mode:           agentcore.ToolExecutionSequential,
		ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
			var request coordination.WaitRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				return agentcore.ToolResult{}, err
			}
			invocation := withEventID(ctx, base, "wait", raw)
			if err := s.Client.Wait(ctx, invocation, request); err != nil {
				return agentcore.ToolResult{}, err
			}
			result := agentcore.TextToolResult("Sleep accepted. This turn is ending now; the durable timer or a higher-priority task heartbeat/message will start a new turn.")
			result.Terminate = true
			return result, nil
		},
	}
	return capability.Resolved{
		Tools:        []agentcore.Tool{delegate, continueIssue, sleep},
		Instructions: []capability.Instruction{{Source: "coordination", Content: `This task uses one Board-based autonomy mode. Use coordinate_delegate to create and assign child Board Issues while you continue working. A failed or budget-exceeded direct child wakes the parent immediately; do not remain idle waiting for other children. Inspect its error, summary and evidence, then explicitly choose: use coordinate_continue to reopen that same Issue with a fresh Execution budget, create a different child with coordinate_delegate, or accept the partial result and continue parent work. Continuing never resets the Task wall-clock budget. Use coordinate_sleep only when no useful action remains; the one-minute system heartbeat wakes a released sleeping or waiting loop but is not injected into active model work. Task messages may still steer or wake you when useful. Use the task Phone to inspect Board and Relay. Messages identify their sender and arrive as steering input; decide yourself whether and when to reply. Issue comments are durable shared context, but replying is a judgment call rather than a runtime obligation.`}},
		Snapshot:     capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: ref.Version},
	}, nil
}

func RegisterDefault(registry *capability.Registry, source Source) error {
	return registry.Register(capability.KindTool, "coordination", source)
}

func withEventID(ctx context.Context, base Invocation, action string, raw json.RawMessage) Invocation {
	callID := ""
	if invocation, ok := agentcore.ToolInvocationFromContext(ctx); ok {
		callID = invocation.Call.ID
	}
	if strings.TrimSpace(callID) == "" {
		digest := sha256.Sum256(append([]byte(action+":"+base.ExecutionID+":"), raw...))
		callID = hex.EncodeToString(digest[:12])
	}
	base.EventID = "coord-tool:" + base.ExecutionID + ":" + action + ":" + callID
	return base
}

var _ capability.Source = Source{}
