package policy

import (
	"context"
	"errors"
	"strings"

	"aegis/agenthost"
	"github.com/z3r2ne/agentcore"
)

type ToolRequest struct {
	Execution agenthost.ExecutionContext
	Turn      int
	Call      agentcore.ToolCall
}

type Decision struct {
	Allow  bool
	Reason string
}

type Authorizer interface {
	AuthorizeTool(context.Context, ToolRequest) (Decision, error)
}

type AuthorizerFunc func(context.Context, ToolRequest) (Decision, error)

func (f AuthorizerFunc) AuthorizeTool(ctx context.Context, request ToolRequest) (Decision, error) {
	return f(ctx, request)
}

// BeforeToolHook converts authorization decisions into agentcore's normal
// blocked tool result. Infrastructure errors remain run errors.
func BeforeToolHook(authorizer Authorizer) func(context.Context, agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
	return ChainBeforeToolHook(nil, authorizer)
}

// ChainBeforeToolHook preserves an existing argument-rewriting/blocking hook
// and authorizes the resulting call afterward.
func ChainBeforeToolHook(previous func(context.Context, agentcore.ToolCallContext) (agentcore.ToolCallDecision, error), authorizer Authorizer) func(context.Context, agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
	return func(ctx context.Context, call agentcore.ToolCallContext) (agentcore.ToolCallDecision, error) {
		var prior agentcore.ToolCallDecision
		if previous != nil {
			decision, err := previous(ctx, call)
			if err != nil || decision.Block {
				return decision, err
			}
			prior = decision
			if len(decision.Arguments) > 0 {
				call.Call.Arguments = append([]byte(nil), decision.Arguments...)
			}
		}
		if authorizer == nil {
			return prior, nil
		}
		execution, _ := agenthost.ExecutionFromContext(ctx)
		decision, err := authorizer.AuthorizeTool(ctx, ToolRequest{Execution: execution, Turn: call.Turn, Call: call.Call})
		if err != nil {
			return agentcore.ToolCallDecision{}, err
		}
		if !decision.Allow {
			reason := strings.TrimSpace(decision.Reason)
			if reason == "" {
				reason = "tool call denied by policy"
			}
			return agentcore.ToolCallDecision{Block: true, Reason: reason}, nil
		}
		return prior, nil
	}
}

// AllowList is a small immutable Authorizer useful for embedded runtimes.
type AllowList map[string]struct{}

func (allowed AllowList) AuthorizeTool(_ context.Context, request ToolRequest) (Decision, error) {
	if allowed == nil {
		return Decision{}, errors.New("policy: nil allowlist")
	}
	_, ok := allowed[request.Call.Name]
	return Decision{Allow: ok, Reason: "tool is not in the execution allowlist"}, nil
}
